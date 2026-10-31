#include "wvrenderer.h"

#include <QFile>
#include <QRandomGenerator>
#include <rhi/qshader.h>

namespace wv {

namespace {
constexpr int kUniformFloats = 44;
constexpr int kGuardW = 8, kGuardH = 5;
} // namespace

Renderer::Renderer() = default;
Renderer::~Renderer() = default;

QShader Renderer::shader(const QString &name)
{
    QFile f(QStringLiteral(":/wv/shaders/") + name + QStringLiteral(".qsb"));
    if (f.open(QIODevice::ReadOnly))
        return QShader::fromSerialized(f.readAll());
    error_ = QStringLiteral("The shader %1 is missing.").arg(name);
    return {};
}

std::unique_ptr<QRhiGraphicsPipeline> Renderer::pipeline(const QShader &vs, const QShader &fs, QRhiShaderResourceBindings *srb, QRhiRenderPassDescriptor *rp, bool additive)
{
    std::unique_ptr<QRhiGraphicsPipeline> p(rhi_->newGraphicsPipeline());
    p->setShaderStages({{QRhiShaderStage::Vertex, vs}, {QRhiShaderStage::Fragment, fs}});
    p->setVertexInputLayout({}); // positions come from the vertex number
    p->setShaderResourceBindings(srb);
    p->setRenderPassDescriptor(rp);
    p->setTopology(QRhiGraphicsPipeline::Triangles);
    p->setCullMode(QRhiGraphicsPipeline::None);
    if (additive) {
        QRhiGraphicsPipeline::TargetBlend b;
        b.enable = true;
        b.srcColor = QRhiGraphicsPipeline::One;
        b.dstColor = QRhiGraphicsPipeline::One;
        b.srcAlpha = QRhiGraphicsPipeline::One;
        b.dstAlpha = QRhiGraphicsPipeline::One;
        p->setTargetBlends({b});
    }
    if (!p->create()) {
        error_ = QStringLiteral("A graphics pipeline could not be made.");
        return nullptr;
    }
    return p;
}

bool Renderer::init(QRhi *rhi, QRhiRenderPassDescriptor *screenPass)
{
    release();
    rhi_ = rhi;
    screenPass_ = screenPass;
    error_.clear();

    const auto makeSampler = [&](QRhiSampler::Filter filter) {
        std::unique_ptr<QRhiSampler> s(rhi_->newSampler(filter, filter, QRhiSampler::None, QRhiSampler::ClampToEdge, QRhiSampler::ClampToEdge));
        s->create();
        return s;
    };
    linear_ = makeSampler(QRhiSampler::Linear);
    nearest_ = makeSampler(QRhiSampler::Nearest);
    audioSampler_ = makeSampler(QRhiSampler::Linear);

    audio_.reset(rhi_->newTexture(QRhiTexture::R8, QSize(kSpectrumBins, kAudioRows)));
    audio_->create();
    black_.reset(rhi_->newTexture(QRhiTexture::RGBA8, QSize(1, 1)));
    black_->create();
    sentHistory_ = -1;
    audioNew_ = true;

    composeUbo_.reset(rhi_->newBuffer(QRhiBuffer::Dynamic, QRhiBuffer::UniformBuffer, kUniformFloats * sizeof(float)));
    composeUbo_->create();

    for (int i = 0; i < 2; i++) {
        guardTex_[i].reset(rhi_->newTexture(QRhiTexture::RGBA8, QSize(kGuardW, kGuardH), 1, QRhiTexture::RenderTarget));
        guardTex_[i]->create();
        guardRt_[i].reset(rhi_->newTextureRenderTarget({guardTex_[i].get()}));
        if (!guardPass_) {
            guardPass_.reset(guardRt_[i]->newCompatibleRenderPassDescriptor());
            offscreenPass_.reset(guardRt_[i]->newCompatibleRenderPassDescriptor());
        }
        guardRt_[i]->setRenderPassDescriptor(guardPass_.get());
        guardRt_[i]->create();
    }
    guardAt_ = 0;

    fullVert_ = shader(QStringLiteral("fullscreen.vert"));
    quadFrag_ = shader(QStringLiteral("quad.frag"));
    const QShader guardFrag = shader(QStringLiteral("guard.frag")), presentFrag = shader(QStringLiteral("present.frag"));
    if (!error_.isEmpty())
        return false;

    const auto stages = QRhiShaderResourceBinding::VertexStage | QRhiShaderResourceBinding::FragmentStage;
    const auto frag = QRhiShaderResourceBinding::FragmentStage;
    const auto bindings = [&](QRhiShaderResourceBindings *s, QRhiTexture *a, QRhiTexture *b, QRhiTexture *g) {
        s->setBindings({
            QRhiShaderResourceBinding::uniformBuffer(0, stages, composeUbo_.get()),
            QRhiShaderResourceBinding::sampledTexture(2, frag, a, linear_.get()),
            QRhiShaderResourceBinding::sampledTexture(3, frag, b, linear_.get()),
            QRhiShaderResourceBinding::sampledTexture(4, frag, g, linear_.get()),
        });
    };
    guardSrb_.reset(rhi_->newShaderResourceBindings());
    bindings(guardSrb_.get(), black_.get(), black_.get(), guardTex_[0].get());
    guardSrb_->create();
    presentSrb_.reset(rhi_->newShaderResourceBindings());
    bindings(presentSrb_.get(), black_.get(), black_.get(), guardTex_[0].get());
    presentSrb_->create();
    guardPipe_ = pipeline(fullVert_, guardFrag, guardSrb_.get(), guardPass_.get(), false);
    presentPipe_ = pipeline(fullVert_, presentFrag, presentSrb_.get(), screenPass_, false);
    return guardPipe_ && presentPipe_;
}

void Renderer::release()
{
    effects_.clear();
    targets_.clear();
    guardPipe_.reset();
    presentPipe_.reset();
    guardSrb_.reset();
    presentSrb_.reset();
    for (int i = 0; i < 2; i++) {
        guardRt_[i].reset();
        guardTex_[i].reset();
    }
    guardPass_.reset();
    offscreenPass_.reset();
    composeUbo_.reset();
    audio_.reset();
    black_.reset();
    linear_.reset();
    nearest_.reset();
    audioSampler_.reset();
    rhi_ = nullptr;
}

void Renderer::setSize(const QSize &size)
{
    size_ = QSize(qMax(2, size.width()), qMax(2, size.height()));
}

bool Renderer::prepare(const EffectInfo &e)
{
    if (effects_.contains(e.id))
        return true;
    auto fx = std::make_shared<Effect>();
    fx->info = e;
    fx->seed = float(QRandomGenerator::global()->generateDouble());
    const QShader main = shader(e.id + QStringLiteral(".frag"));
    QShader quadV;
    if (e.quads)
        quadV = shader(e.id + QStringLiteral(".quad.vert"));
    if (!main.isValid() || (e.quads && !quadV.isValid()))
        return false;
    fx->ubo.reset(rhi_->newBuffer(QRhiBuffer::Dynamic, QRhiBuffer::UniformBuffer, kUniformFloats * sizeof(float)));
    fx->ubo->create();
    const auto stages = QRhiShaderResourceBinding::VertexStage | QRhiShaderResourceBinding::FragmentStage;
    fx->srb[0].reset(rhi_->newShaderResourceBindings());
    fx->srb[0]->setBindings({
        QRhiShaderResourceBinding::uniformBuffer(0, stages, fx->ubo.get()),
        QRhiShaderResourceBinding::sampledTexture(1, stages, audio_.get(), audioSampler_.get()),
        QRhiShaderResourceBinding::sampledTexture(2, QRhiShaderResourceBinding::FragmentStage, black_.get(), linear_.get()),
    });
    fx->srb[0]->create();
    fx->pipe = pipeline(fullVert_, main, fx->srb[0].get(), offscreenPass_.get(), false);
    if (!fx->pipe)
        return false;
    if (e.quads) {
        fx->quadSrb.reset(rhi_->newShaderResourceBindings());
        fx->quadSrb->setBindings({
            QRhiShaderResourceBinding::uniformBuffer(0, stages, fx->ubo.get()),
            QRhiShaderResourceBinding::sampledTexture(1, stages, audio_.get(), audioSampler_.get()),
        });
        fx->quadSrb->create();
        fx->quadPipe = pipeline(quadV, quadFrag_, fx->quadSrb.get(), offscreenPass_.get(), true);
        if (!fx->quadPipe)
            return false;
    }
    effects_.insert(e.id, fx);
    return true;
}

void Renderer::releaseTarget(const QString &id)
{
    targets_.remove(id);
}

Renderer::Target &Renderer::target(const QString &id, bool feedback, QRhiResourceUpdateBatch *u, QRhiCommandBuffer *cb)
{
    auto it = targets_.find(id);
    if (it != targets_.end() && (*it)->size == size_)
        return **it;
    auto t = std::make_shared<Target>();
    t->size = size_;
    // Copied up from an old size below: a feedback effect keeps what it was building on.
    const int n = feedback ? 2 : 1;
    for (int i = 0; i < n; i++) {
        t->tex[i].reset(rhi_->newTexture(QRhiTexture::RGBA8, size_, 1, QRhiTexture::RenderTarget | QRhiTexture::UsedAsTransferSource));
        t->tex[i]->create();
        t->rt[i].reset(rhi_->newTextureRenderTarget({t->tex[i].get()}));
        t->rt[i]->setRenderPassDescriptor(offscreenPass_.get());
        t->rt[i]->create();
    }
    Q_UNUSED(u);
    Q_UNUSED(cb);
    targets_.insert(id, t);
    return *t;
}

void Renderer::fillUniforms(float *u, const DrawInput &f, float seed) const
{
    const AudioFrame &a = *f.audio;
    u[0] = float(f.time);
    u[1] = float(f.flow);
    u[2] = float(f.dt);
    u[3] = float(a.beatCount);
    u[4] = float(a.bass);
    u[5] = float(a.mid);
    u[6] = float(a.treble);
    u[7] = float(a.level);
    for (int i = 0; i < 4; i++)
        u[8 + i] = float(a.beatAges[size_t(i)]);
    u[12] = float(size_.width());
    u[13] = float(size_.height());
    u[14] = float(size_.width()) / float(size_.height());
    u[15] = float(a.historyRow);
    u[16] = float(f.intensity);
    u[17] = f.calm ? 1.f : 0.f;
    u[18] = seed;
    u[19] = float(a.beat * (f.calm ? 0.35 : 1.0) * f.intensity);
    // Direct3D, Metal and Vulkan read rendered textures from the top down, and Vulkan's clip
    // space is upside down as well. The shaders always think in "y up" and are told this.
    u[20] = rhi_->isYUpInFramebuffer() ? 0.f : 1.f;
    u[21] = rhi_->isYUpInNDC() ? 1.f : -1.f;
    u[22] = f.guard.deadBand;
    u[23] = f.guard.rise;
    u[24] = float(f.mix);
    u[25] = float(f.mixStyle);
    u[26] = 0;
    u[27] = 0;
    for (size_t k = 0; k < 4; k++) {
        for (size_t i = 0; i < 3; i++)
            u[28 + 4 * k + i] = f.palette[k][i];
        u[28 + 4 * k + 3] = 0;
    }
}

void Renderer::drawEffect(QRhiCommandBuffer *cb, const QString &id, const DrawInput &f, QRhiResourceUpdateBatch *&u)
{
    const auto it = effects_.constFind(id);
    if (it == effects_.constEnd())
        return;
    Effect &e = **it;
    Target &t = target(id, e.info.feedback, u, cb);
    const int out = e.info.feedback ? 1 - t.at : 0;

    float uni[kUniformFloats];
    fillUniforms(uni, f, e.seed);
    u->updateDynamicBuffer(e.ubo.get(), 0, sizeof(uni), uni);

    // The texture to read from changes each frame, so the bindings do.
    const auto stages = QRhiShaderResourceBinding::VertexStage | QRhiShaderResourceBinding::FragmentStage;
    e.srb[0]->setBindings({
        QRhiShaderResourceBinding::uniformBuffer(0, stages, e.ubo.get()),
        QRhiShaderResourceBinding::sampledTexture(1, stages, audio_.get(), audioSampler_.get()),
        QRhiShaderResourceBinding::sampledTexture(2, QRhiShaderResourceBinding::FragmentStage, e.info.feedback ? t.tex[t.at].get() : black_.get(), linear_.get()),
    });
    e.srb[0]->updateResources();

    cb->beginPass(t.rt[out].get(), Qt::transparent, {1.0f, 0}, u);
    u = nullptr;
    cb->setGraphicsPipeline(e.pipe.get());
    cb->setViewport(QRhiViewport(0, 0, float(t.size.width()), float(t.size.height())));
    cb->setShaderResources(e.srb[0].get());
    cb->draw(3);
    if (e.quadPipe) {
        cb->setGraphicsPipeline(e.quadPipe.get());
        cb->setViewport(QRhiViewport(0, 0, float(t.size.width()), float(t.size.height())));
        cb->setShaderResources(e.quadSrb.get());
        cb->draw(quint32(e.info.quads) * 6);
    }
    cb->endPass();
    if (e.info.feedback)
        t.at = out;
    u = rhi_->nextResourceUpdateBatch();
}

QRhiTexture *Renderer::textureOf(const QString &id) const
{
    const auto it = targets_.constFind(id);
    return it == targets_.constEnd() ? black_.get() : (*it)->tex[(*it)->at].get();
}

void Renderer::draw(QRhiCommandBuffer *cb, QRhiRenderTarget *screen, const DrawInput &f, const QColor &clear)
{
    QRhiResourceUpdateBatch *u = rhi_->nextResourceUpdateBatch();
    if (audioNew_) {
        static const uint32_t blackPixel = 0xff000000u;
        u->uploadTexture(black_.get(), QRhiTextureUploadDescription({0, 0, QRhiTextureSubresourceUploadDescription(&blackPixel, sizeof(blackPixel))}));
        std::vector<uint8_t> zero(size_t(kSpectrumBins) * kAudioRows, 0);
        u->uploadTexture(audio_.get(), QRhiTextureUploadDescription({0, 0, QRhiTextureSubresourceUploadDescription(zero.data(), quint32(zero.size()))}));
        audioNew_ = false;
    }
    const AudioFrame &a = *f.audio;
    // Only what's new: the top four rows, and one row of history.
    QRhiTextureSubresourceUploadDescription rows(a.texture.data(), quint32(size_t(kSpectrumBins) * 4));
    rows.setSourceSize(QSize(kSpectrumBins, 4));
    rows.setDataStride(kSpectrumBins);
    QVector<QRhiTextureUploadEntry> entries{{0, 0, rows}};
    if (a.historyRow != sentHistory_) {
        const int row = 4 + a.historyRow;
        QRhiTextureSubresourceUploadDescription hist(a.texture.data() + size_t(row) * kSpectrumBins, quint32(kSpectrumBins));
        hist.setSourceSize(QSize(kSpectrumBins, 1));
        hist.setDataStride(kSpectrumBins);
        hist.setDestinationTopLeft(QPoint(0, row));
        entries.push_back({0, 0, hist});
        sentHistory_ = a.historyRow;
    }
    QRhiTextureUploadDescription up;
    up.setEntries(entries.cbegin(), entries.cend());
    u->uploadTexture(audio_.get(), up);

    drawEffect(cb, f.a, f, u);
    if (!f.b.isEmpty())
        drawEffect(cb, f.b, f, u);

    float uni[kUniformFloats];
    fillUniforms(uni, f, 0);
    u->updateDynamicBuffer(composeUbo_.get(), 0, sizeof(uni), uni);

    QRhiTexture *ta = textureOf(f.a), *tb = f.b.isEmpty() ? black_.get() : textureOf(f.b);
    const auto stages = QRhiShaderResourceBinding::VertexStage | QRhiShaderResourceBinding::FragmentStage;
    const auto frag = QRhiShaderResourceBinding::FragmentStage;
    const auto setCompose = [&](QRhiShaderResourceBindings *s, QRhiTexture *g) {
        s->setBindings({
            QRhiShaderResourceBinding::uniformBuffer(0, stages, composeUbo_.get()),
            QRhiShaderResourceBinding::sampledTexture(2, frag, ta, linear_.get()),
            QRhiShaderResourceBinding::sampledTexture(3, frag, tb, linear_.get()),
            QRhiShaderResourceBinding::sampledTexture(4, frag, g, linear_.get()),
        });
        s->updateResources();
    };

    // The guard: reads what's about to be shown and last frame's memory.
    const int next = 1 - guardAt_;
    setCompose(guardSrb_.get(), guardTex_[guardAt_].get());
    cb->beginPass(guardRt_[next].get(), Qt::black, {1.0f, 0}, u);
    cb->setGraphicsPipeline(guardPipe_.get());
    cb->setViewport(QRhiViewport(0, 0, kGuardW, kGuardH));
    cb->setShaderResources(guardSrb_.get());
    cb->draw(3);
    cb->endPass();
    guardAt_ = next;

    setCompose(presentSrb_.get(), guardTex_[guardAt_].get());
    cb->beginPass(screen, clear, {1.0f, 0});
    cb->setGraphicsPipeline(presentPipe_.get());
    const QSize out = screen->pixelSize();
    cb->setViewport(QRhiViewport(0, 0, float(out.width()), float(out.height())));
    cb->setShaderResources(presentSrb_.get());
    cb->draw(3);
    cb->endPass();
}

} // namespace wv
