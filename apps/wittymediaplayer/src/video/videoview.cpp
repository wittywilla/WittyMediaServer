#include "videoview.h"

#include <QFile>
#include <QMatrix4x4>
#include <QResizeEvent>
#include <rhi/qrhi.h>
#include <rhi/qshader.h>
#include <algorithm>

namespace wmp {

namespace {

QShader loadShader(const QString &name)
{
    QFile f(QStringLiteral(":/shaders/") + name);
    if (f.open(QIODevice::ReadOnly))
        return QShader::fromSerialized(f.readAll());
    return {};
}

// A quad from 0,0 to 1,1, with its texture coordinates.
const float kQuad[] = {
    0, 0, 0, 0,
    0, 1, 0, 1,
    1, 0, 1, 0,
    1, 1, 1, 1,
};

constexpr int kUniformBytes = 64 + 64 + 16;

} // namespace

VideoView::VideoView(QWidget *parent) : QRhiWidget(parent)
{
    setFocusPolicy(Qt::StrongFocus);
    setMinimumSize(320, 180);
    setAccessibleName(QStringLiteral("Video"));
}

VideoView::~VideoView() = default;

void VideoView::presentFrame(const FramePtr &frame)
{
    bool post = false;
    {
        QMutexLocker l(&mu_);
        frame_ = frame;
        frameDirty_ = true;
        // One waiting update is enough: a slow screen shows the newest picture.
        if (!updatePending_) {
            updatePending_ = true;
            post = true;
        }
    }
    if (post)
        QMetaObject::invokeMethod(this, &VideoView::onNewFrame, Qt::QueuedConnection);
}

void VideoView::onNewFrame()
{
    double was;
    {
        QMutexLocker l(&mu_);
        updatePending_ = false;
        was = lastAspect_;
    }
    const double now = frameAspect();
    if (std::abs(now - was) > 0.001) {
        QMutexLocker l(&mu_);
        lastAspect_ = now;
        l.unlock();
        emit pictureRectChanged();
    }
    update();
}

void VideoView::clearPicture()
{
    {
        QMutexLocker l(&mu_);
        frame_.reset();
        frameDirty_ = false;
        hasPicture_ = false;
    }
    update();
}

FramePtr VideoView::currentFrame() const
{
    QMutexLocker l(&mu_);
    return frame_;
}

void VideoView::setAspect(const QString &aspect)
{
    {
        QMutexLocker l(&mu_);
        if (aspect_ == aspect)
            return;
        aspect_ = aspect;
    }
    emit pictureRectChanged();
    update();
}

void VideoView::setOverlays(const QVector<Overlay> &overlays)
{
    {
        QMutexLocker l(&mu_);
        overlays_ = overlays;
        overlaysDirty_ = true;
    }
    update();
}

double VideoView::frameAspect() const
{
    QMutexLocker l(&mu_);
    if (aspect_ == QLatin1String("4:3"))
        return 4.0 / 3.0;
    if (aspect_ == QLatin1String("16:9"))
        return 16.0 / 9.0;
    if (frame_ && frame_->width > 0 && frame_->height > 0) {
        double sar = frame_->sample_aspect_ratio.num > 0 && frame_->sample_aspect_ratio.den > 0 ? av_q2d(frame_->sample_aspect_ratio) : 1.0;
        return double(frame_->width) * sar / frame_->height;
    }
    return lastAspect_;
}

QRectF VideoView::fit(double aspect, const QRectF &area)
{
    if (aspect <= 0 || area.isEmpty())
        return area;
    double w = area.width(), h = w / aspect;
    if (h > area.height()) {
        h = area.height();
        w = h * aspect;
    }
    return QRectF(area.x() + (area.width() - w) / 2, area.y() + (area.height() - h) / 2, w, h);
}

QRectF VideoView::pictureRect() const
{
    return fit(frameAspect(), QRectF(rect()));
}

void VideoView::resizeEvent(QResizeEvent *e)
{
    QRhiWidget::resizeEvent(e);
    emit pictureRectChanged();
}

QMatrix4x4 VideoView::colorMatrix(const AVFrame *f)
{
    // Kr and Kb for the picture's colour system.
    double kr = 0.2126, kb = 0.0722; // BT.709
    switch (f->colorspace) {
    case AVCOL_SPC_BT470BG:
    case AVCOL_SPC_SMPTE170M:
        kr = 0.299;
        kb = 0.114;
        break;
    case AVCOL_SPC_BT2020_NCL:
    case AVCOL_SPC_BT2020_CL:
        kr = 0.2627;
        kb = 0.0593;
        break;
    case AVCOL_SPC_BT709:
        break;
    default:
        // Unmarked: standard definition is BT.601.
        if (f->height < 720) {
            kr = 0.299;
            kb = 0.114;
        }
    }
    const double kg = 1.0 - kr - kb;
    const bool full = f->color_range == AVCOL_RANGE_JPEG || f->format == AV_PIX_FMT_YUVJ420P;
    const double yScale = full ? 1.0 : 255.0 / 219.0;
    const double cScale = full ? 1.0 : 255.0 / 224.0;
    const double yOff = full ? 0.0 : 16.0 / 255.0;
    const double cOff = 128.0 / 255.0;
    const double rv = cScale * 2 * (1 - kr);
    const double bu = cScale * 2 * (1 - kb);
    const double gu = -cScale * 2 * (1 - kb) * kb / kg;
    const double gv = -cScale * 2 * (1 - kr) * kr / kg;
    QMatrix4x4 m(float(yScale), 0, float(rv), float(-yScale * yOff - rv * cOff),
                 float(yScale), float(gu), float(gv), float(-yScale * yOff - (gu + gv) * cOff),
                 float(yScale), float(bu), 0, float(-yScale * yOff - bu * cOff),
                 0, 0, 0, 1);
    return m;
}

void VideoView::initialize(QRhiCommandBuffer *)
{
    if (rhi_ != rhi()) {
        // A new graphics device: everything made on the old one is gone.
        releaseResources();
        rhi_ = rhi();
    }
    if (vbuf_)
        return;
    vbuf_.reset(rhi_->newBuffer(QRhiBuffer::Immutable, QRhiBuffer::VertexBuffer, sizeof(kQuad)));
    vbuf_->create();
    ubuf_.reset(rhi_->newBuffer(QRhiBuffer::Dynamic, QRhiBuffer::UniformBuffer, kUniformBytes));
    ubuf_->create();
    sampler_.reset(rhi_->newSampler(QRhiSampler::Linear, QRhiSampler::Linear, QRhiSampler::None, QRhiSampler::ClampToEdge, QRhiSampler::ClampToEdge));
    sampler_->create();
    overlaySampler_.reset(rhi_->newSampler(QRhiSampler::Linear, QRhiSampler::Linear, QRhiSampler::None, QRhiSampler::ClampToEdge, QRhiSampler::ClampToEdge));
    overlaySampler_->create();
    emptyTex_.reset(rhi_->newTexture(QRhiTexture::R8, QSize(1, 1)));
    emptyTex_->create();
    vertsUploaded_ = false;
    hasPicture_ = false;
    ensurePipelines();
}

void VideoView::ensurePipelines()
{
    srb_.reset(rhi_->newShaderResourceBindings());
    auto stage = QRhiShaderResourceBinding::VertexStage | QRhiShaderResourceBinding::FragmentStage;
    QRhiTexture *y = texY_ ? texY_.get() : emptyTex_.get();
    QRhiTexture *u = texU_ ? texU_.get() : emptyTex_.get();
    QRhiTexture *v = texV_ ? texV_.get() : emptyTex_.get();
    srb_->setBindings({
        QRhiShaderResourceBinding::uniformBuffer(0, stage, ubuf_.get()),
        QRhiShaderResourceBinding::sampledTexture(1, QRhiShaderResourceBinding::FragmentStage, y, sampler_.get()),
        QRhiShaderResourceBinding::sampledTexture(2, QRhiShaderResourceBinding::FragmentStage, u, sampler_.get()),
        QRhiShaderResourceBinding::sampledTexture(3, QRhiShaderResourceBinding::FragmentStage, v, sampler_.get()),
    });
    srb_->create();
    if (videoPipe_)
        return;

    QRhiVertexInputLayout layout;
    layout.setBindings({{4 * sizeof(float)}});
    layout.setAttributes({{0, 0, QRhiVertexInputAttribute::Float2, 0}, {0, 1, QRhiVertexInputAttribute::Float2, 2 * sizeof(float)}});

    videoPipe_.reset(rhi_->newGraphicsPipeline());
    videoPipe_->setTopology(QRhiGraphicsPipeline::TriangleStrip);
    videoPipe_->setShaderStages({{QRhiShaderStage::Vertex, loadShader(QStringLiteral("quad.vert.qsb"))}, {QRhiShaderStage::Fragment, loadShader(QStringLiteral("video.frag.qsb"))}});
    videoPipe_->setVertexInputLayout(layout);
    videoPipe_->setShaderResourceBindings(srb_.get());
    videoPipe_->setRenderPassDescriptor(renderTarget()->renderPassDescriptor());
    videoPipe_->create();

    // A layout pipeline made once serves every overlay's own bindings.
    std::unique_ptr<QRhiShaderResourceBindings> proto(rhi_->newShaderResourceBindings());
    proto->setBindings({
        QRhiShaderResourceBinding::uniformBuffer(0, stage, ubuf_.get()),
        QRhiShaderResourceBinding::sampledTexture(1, QRhiShaderResourceBinding::FragmentStage, emptyTex_.get(), overlaySampler_.get()),
    });
    proto->create();
    overlayPipe_.reset(rhi_->newGraphicsPipeline());
    overlayPipe_->setTopology(QRhiGraphicsPipeline::TriangleStrip);
    overlayPipe_->setShaderStages({{QRhiShaderStage::Vertex, loadShader(QStringLiteral("quad.vert.qsb"))}, {QRhiShaderStage::Fragment, loadShader(QStringLiteral("overlay.frag.qsb"))}});
    overlayPipe_->setVertexInputLayout(layout);
    overlayPipe_->setShaderResourceBindings(proto.get());
    QRhiGraphicsPipeline::TargetBlend blend;
    blend.enable = true;
    blend.srcColor = QRhiGraphicsPipeline::One;
    blend.dstColor = QRhiGraphicsPipeline::OneMinusSrcAlpha;
    blend.srcAlpha = QRhiGraphicsPipeline::One;
    blend.dstAlpha = QRhiGraphicsPipeline::OneMinusSrcAlpha;
    overlayPipe_->setTargetBlends({blend});
    overlayPipe_->setRenderPassDescriptor(renderTarget()->renderPassDescriptor());
    overlayPipe_->create();
}

void VideoView::uploadFrame(QRhiResourceUpdateBatch *u, const AVFrame *f)
{
    const bool deep = f->format == AV_PIX_FMT_P010;
    const bool semi = f->format == AV_PIX_FMT_NV12 || deep;
    const QRhiTexture::Format fy = deep ? QRhiTexture::R16 : QRhiTexture::R8;
    const QRhiTexture::Format fu = semi ? (deep ? QRhiTexture::RG16 : QRhiTexture::RG8) : QRhiTexture::R8;
    const QSize sy(f->width, f->height), su((f->width + 1) / 2, (f->height + 1) / 2);
    bool rebuilt = false;
    auto ensure = [&](std::unique_ptr<QRhiTexture> &t, QRhiTexture::Format fmt, QSize size) {
        if (!t || t->format() != fmt || t->pixelSize() != size) {
            t.reset(rhi_->newTexture(fmt, size));
            t->create();
            rebuilt = true;
        }
    };
    ensure(texY_, fy, sy);
    ensure(texU_, fu, su);
    if (semi) {
        if (texV_) {
            texV_.reset();
            rebuilt = true;
        }
    } else {
        ensure(texV_, QRhiTexture::R8, su);
    }
    if (rebuilt)
        ensurePipelines();

    auto plane = [&](QRhiTexture *t, int i, QSize size, int bytesPerPixel) {
        QRhiTextureSubresourceUploadDescription d(f->data[i], f->linesize[i] * size.height());
        d.setDataStride(quint32(f->linesize[i]));
        d.setSourceSize(QSize(size.width(), size.height()));
        (void)bytesPerPixel;
        u->uploadTexture(t, QRhiTextureUploadDescription({0, 0, d}));
    };
    plane(texY_.get(), 0, sy, deep ? 2 : 1);
    plane(texU_.get(), 1, su, semi ? (deep ? 4 : 2) : 1);
    if (!semi)
        plane(texV_.get(), 2, su, 1);

    semiPlanar_ = semi;
    color_ = colorMatrix(f);
    hdr_ = f->color_trc == AVCOL_TRC_SMPTE2084 || f->color_trc == AVCOL_TRC_ARIB_STD_B67;
    hasPicture_ = true;
}

QMatrix4x4 VideoView::mvpFor(const QRectF &r) const
{
    const QSize px = colorTexture() ? colorTexture()->pixelSize() : size() * devicePixelRatioF();
    const double dpr = devicePixelRatioF();
    QMatrix4x4 m = rhi_->clipSpaceCorrMatrix();
    m.ortho(0, float(px.width()), float(px.height()), 0, -1, 1);
    m.translate(float(r.x() * dpr), float(r.y() * dpr));
    m.scale(float(r.width() * dpr), float(r.height() * dpr));
    return m;
}

void VideoView::ensureOverlayResources(QRhiResourceUpdateBatch *u)
{
    QVector<Overlay> wanted;
    {
        QMutexLocker l(&mu_);
        if (!overlaysDirty_)
            return;
        overlaysDirty_ = false;
        wanted = overlays_;
    }
    std::vector<std::unique_ptr<DrawnOverlay>> next;
    next.reserve(size_t(wanted.size()));
    for (const Overlay &o : wanted) {
        // A picture already on the card stays there: moving a highlight costs nothing.
        auto found = std::find_if(drawn_.begin(), drawn_.end(), [&](const auto &d) { return d && o.key != 0 && d->key == o.key; });
        if (found != drawn_.end()) {
            (*found)->rect = o.rect;
            next.push_back(std::move(*found));
            continue;
        }
        if (o.image.isNull())
            continue;
        auto d = std::make_unique<DrawnOverlay>();
        d->key = o.key;
        d->rect = o.rect;
        const QImage img = o.image.format() == QImage::Format_RGBA8888_Premultiplied ? o.image : o.image.convertToFormat(QImage::Format_RGBA8888_Premultiplied);
        d->tex.reset(rhi_->newTexture(QRhiTexture::RGBA8, img.size()));
        d->tex->create();
        u->uploadTexture(d->tex.get(), img);
        d->ubuf.reset(rhi_->newBuffer(QRhiBuffer::Dynamic, QRhiBuffer::UniformBuffer, kUniformBytes));
        d->ubuf->create();
        d->srb.reset(rhi_->newShaderResourceBindings());
        d->srb->setBindings({
            QRhiShaderResourceBinding::uniformBuffer(0, QRhiShaderResourceBinding::VertexStage | QRhiShaderResourceBinding::FragmentStage, d->ubuf.get()),
            QRhiShaderResourceBinding::sampledTexture(1, QRhiShaderResourceBinding::FragmentStage, d->tex.get(), overlaySampler_.get()),
        });
        d->srb->create();
        next.push_back(std::move(d));
    }
    drawn_ = std::move(next);
}

void VideoView::render(QRhiCommandBuffer *cb)
{
    QRhiResourceUpdateBatch *u = rhi_->nextResourceUpdateBatch();
    if (!vertsUploaded_) {
        u->uploadStaticBuffer(vbuf_.get(), kQuad);
        vertsUploaded_ = true;
    }
    FramePtr f;
    bool dirty;
    {
        QMutexLocker l(&mu_);
        dirty = frameDirty_;
        frameDirty_ = false;
        f = frame_;
    }
    if (f && dirty)
        uploadFrame(u, f.get());
    ensureOverlayResources(u);

    const QRectF pic = pictureRect();
    const float params[4] = {semiPlanar_ ? 1.f : 0.f, hdr_ ? 1.f : 0.f, 0.f, 0.f};
    if (hasPicture_) {
        QMatrix4x4 mvp = mvpFor(pic);
        u->updateDynamicBuffer(ubuf_.get(), 0, 64, mvp.constData());
        u->updateDynamicBuffer(ubuf_.get(), 64, 64, color_.constData());
        u->updateDynamicBuffer(ubuf_.get(), 128, 16, params);
    }
    for (const auto &d : drawn_) {
        const QRectF r(pic.x() + d->rect.x() * pic.width(), pic.y() + d->rect.y() * pic.height(), d->rect.width() * pic.width(), d->rect.height() * pic.height());
        QMatrix4x4 mvp = mvpFor(r);
        u->updateDynamicBuffer(d->ubuf.get(), 0, 64, mvp.constData());
    }

    const QColor black(0, 0, 0);
    cb->beginPass(renderTarget(), black, {1.0f, 0}, u);
    const QSize px = renderTarget()->pixelSize();
    cb->setViewport({0, 0, float(px.width()), float(px.height())});
    const QRhiCommandBuffer::VertexInput vb(vbuf_.get(), 0);
    if (hasPicture_) {
        cb->setGraphicsPipeline(videoPipe_.get());
        cb->setShaderResources(srb_.get());
        cb->setVertexInput(0, 1, &vb);
        cb->draw(4);
    }
    if (!drawn_.empty()) {
        cb->setGraphicsPipeline(overlayPipe_.get());
        cb->setVertexInput(0, 1, &vb);
        for (const auto &d : drawn_) {
            cb->setShaderResources(d->srb.get());
            cb->draw(4);
        }
    }
    cb->endPass();
}

void VideoView::releaseResources()
{
    drawn_.clear();
    videoPipe_.reset();
    overlayPipe_.reset();
    srb_.reset();
    texY_.reset();
    texU_.reset();
    texV_.reset();
    emptyTex_.reset();
    sampler_.reset();
    overlaySampler_.reset();
    ubuf_.reset();
    vbuf_.reset();
    hasPicture_ = false;
    QMutexLocker l(&mu_);
    frameDirty_ = frame_ != nullptr;
    overlaysDirty_ = true;
}

} // namespace wmp
