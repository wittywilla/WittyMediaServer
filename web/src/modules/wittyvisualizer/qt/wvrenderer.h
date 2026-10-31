// Draws effects with QRhi: gl.ts for the desktop. Each effect draws into textures of its own;
// the guard and present passes turn those into the picture, so nothing reaches the screen
// without passing the flash guard. It isn't tied to a widget, so a test can run it with no
// window at all.
#pragma once

#include "wvanalysis.h"
#include "wvengine.h"

#include <QHash>
#include <QSize>
#include <memory>
#include <rhi/qrhi.h>

namespace wv {

struct DrawInput {
    const AudioFrame *audio = nullptr;
    double time = 0, flow = 0, dt = 1.0 / 60;
    double intensity = 1;
    bool calm = false;
    std::array<std::array<float, 3>, 4> palette{};
    GuardLimits guard = kGuard;
    QString a; // on screen
    QString b; // coming in
    double mix = 0;
    int mixStyle = 0;
};

class Renderer {
public:
    Renderer();
    ~Renderer();

    // Call again when the graphics API or the target's kind changes. False if the shaders
    // won't load: the problem is in error().
    bool init(QRhi *rhi, QRhiRenderPassDescriptor *screenPass);
    // Frees everything the graphics card holds.
    void release();
    QString error() const { return error_; }

    // The size effects draw at; the screen can be larger.
    void setSize(const QSize &size);
    QSize size() const { return size_; }
    // Builds what an effect needs. The first time, this compiles shaders.
    bool prepare(const EffectInfo &e);
    // Lets go of an effect's textures once it's off the screen.
    void releaseTarget(const QString &id);
    // Records one frame, ending in a pass on screen.
    void draw(QRhiCommandBuffer *cb, QRhiRenderTarget *screen, const DrawInput &f, const QColor &clear = Qt::black);

private:
    struct Target {
        QSize size;
        std::unique_ptr<QRhiTexture> tex[2];
        std::unique_ptr<QRhiTextureRenderTarget> rt[2];
        int at = 0;
    };
    struct Effect {
        EffectInfo info;
        float seed = 0;
        std::unique_ptr<QRhiBuffer> ubo;
        std::unique_ptr<QRhiShaderResourceBindings> srb[2];
        std::unique_ptr<QRhiShaderResourceBindings> quadSrb;
        std::unique_ptr<QRhiGraphicsPipeline> pipe, quadPipe;
    };

    QShader shader(const QString &name);
    std::unique_ptr<QRhiGraphicsPipeline> pipeline(const QShader &vs, const QShader &fs, QRhiShaderResourceBindings *srb, QRhiRenderPassDescriptor *rp, bool additive);
    Target &target(const QString &id, bool feedback, QRhiResourceUpdateBatch *u, QRhiCommandBuffer *cb);
    void fillUniforms(float *out, const DrawInput &f, float seed) const;
    void drawEffect(QRhiCommandBuffer *cb, const QString &id, const DrawInput &f, QRhiResourceUpdateBatch *&u);
    QRhiTexture *textureOf(const QString &id) const;

    QRhi *rhi_ = nullptr;
    QRhiRenderPassDescriptor *screenPass_ = nullptr;
    QString error_;
    QSize size_{2, 2};
    bool audioNew_ = true;

    QShader fullVert_, quadFrag_;
    std::unique_ptr<QRhiBuffer> composeUbo_;
    std::unique_ptr<QRhiSampler> linear_, nearest_, audioSampler_;
    std::unique_ptr<QRhiTexture> audio_, black_;
    std::unique_ptr<QRhiTexture> guardTex_[2];
    std::unique_ptr<QRhiTextureRenderTarget> guardRt_[2];
    std::unique_ptr<QRhiRenderPassDescriptor> offscreenPass_, guardPass_;
    std::unique_ptr<QRhiShaderResourceBindings> guardSrb_, presentSrb_;
    std::unique_ptr<QRhiGraphicsPipeline> guardPipe_, presentPipe_;
    int guardAt_ = 0;
    QHash<QString, std::shared_ptr<Effect>> effects_;
    QHash<QString, std::shared_ptr<Target>> targets_;
};

} // namespace wv
