// Shows decoded pictures with menus and subtitles over them, on the graphics card.
// QRhi picks the system's own graphics API (Direct3D 11 on Windows), so nothing
// here is tied to one platform.
#pragma once

#include "../media/player.h"

#include <QImage>
#include <QMatrix4x4>
#include <QMutex>
#include <QRhiWidget>
#include <QVector>
#include <memory>
#include <rhi/qrhi.h>


namespace wmp {

// A picture to draw over the video: rect is in 0..1 of the picture's area.
struct Overlay {
    QImage image;
    QRectF rect;
    uint64_t key = 0; // the same key means the same picture
};

class VideoView : public QRhiWidget, public VideoSink {
    Q_OBJECT
public:
    explicit VideoView(QWidget *parent = nullptr);
    ~VideoView() override;

    void presentFrame(const FramePtr &frame) override;
    void clearPicture();
    FramePtr currentFrame() const;

    // Forces the picture's shape (a disc says what it is); empty uses the frame's.
    void setAspect(const QString &aspect);
    void setOverlays(const QVector<Overlay> &overlays);

    // Where the picture is, in widget coordinates.
    QRectF pictureRect() const;

    // The picture rectangle for a frame of this shape in an area. Public for tests.
    static QRectF fit(double aspect, const QRectF &area);

    // Colour conversion for a frame. Public for tests.
    static QMatrix4x4 colorMatrix(const AVFrame *f);

signals:
    void pictureRectChanged();

protected:
    void initialize(QRhiCommandBuffer *cb) override;
    void render(QRhiCommandBuffer *cb) override;
    void releaseResources() override;
    void resizeEvent(QResizeEvent *e) override;

private slots:
    void onNewFrame();

private:
    void ensurePipelines();
    void uploadFrame(QRhiResourceUpdateBatch *u, const AVFrame *f);
    void ensureOverlayResources(QRhiResourceUpdateBatch *u);
    QMatrix4x4 mvpFor(const QRectF &r) const;
    double frameAspect() const;

    mutable QMutex mu_;
    FramePtr frame_;
    bool frameDirty_ = false;
    bool updatePending_ = false;
    QString aspect_;
    double lastAspect_ = 16.0 / 9.0;
    QVector<Overlay> overlays_;
    bool overlaysDirty_ = false;

    struct DrawnOverlay {
        uint64_t key = 0;
        QRectF rect;
        std::unique_ptr<QRhiTexture> tex;
        std::unique_ptr<QRhiBuffer> ubuf;
        std::unique_ptr<QRhiShaderResourceBindings> srb;
    };
    std::vector<std::unique_ptr<DrawnOverlay>> drawn_;

    QRhi *rhi_ = nullptr;
    std::unique_ptr<QRhiBuffer> vbuf_, ubuf_;
    std::unique_ptr<QRhiSampler> sampler_, overlaySampler_;
    std::unique_ptr<QRhiTexture> texY_, texU_, texV_, emptyTex_;
    std::unique_ptr<QRhiShaderResourceBindings> srb_;
    std::unique_ptr<QRhiGraphicsPipeline> videoPipe_, overlayPipe_;
    QRhiTexture::Format fmtY_ = QRhiTexture::UnknownFormat, fmtU_ = QRhiTexture::UnknownFormat;
    QSize sizeY_, sizeU_;
    bool semiPlanar_ = false;
    bool hdr_ = false;
    QMatrix4x4 color_;
    bool hasPicture_ = false;
    bool vertsUploaded_ = false;
};

} // namespace wmp
