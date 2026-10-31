#include "visualizer.h"

#include "media/audiooutput.h"
#include "wvengine.h"

#include <QMouseEvent>
#include <QPainter>
#include <QPainterPath>
#include <algorithm>
#include <cmath>
#include <complex>

namespace wmp {

namespace {

constexpr int kFrames = 1024;
constexpr int kBands = 28;
constexpr int kFrameMs = 33;
constexpr double kPi = 3.14159265358979323846;
constexpr int kTilesX = 16, kTilesY = 9;

void fft(std::vector<std::complex<float>> &a)
{
    const size_t n = a.size();
    for (size_t i = 1, j = 0; i < n; i++) {
        size_t bit = n >> 1;
        for (; j & bit; bit >>= 1)
            j ^= bit;
        j ^= bit;
        if (i < j)
            std::swap(a[i], a[j]);
    }
    for (size_t len = 2; len <= n; len <<= 1) {
        const double ang = -2 * kPi / double(len);
        const std::complex<float> step(float(std::cos(ang)), float(std::sin(ang)));
        for (size_t i = 0; i < n; i += len) {
            std::complex<float> w(1, 0);
            for (size_t k = 0; k < len / 2; k++) {
                const std::complex<float> u = a[i + k], v = a[i + k + len / 2] * w;
                a[i + k] = u + v;
                a[i + k + len / 2] = u - v;
                w *= step;
            }
        }
    }
}

QColor mix(const QColor &a, const QColor &b, float t)
{
    t = std::clamp(t, 0.f, 1.f);
    return QColor::fromRgbF(a.redF() + (b.redF() - a.redF()) * t, a.greenF() + (b.greenF() - a.greenF()) * t, a.blueF() + (b.blueF() - a.blueF()) * t);
}

} // namespace

void spectrumBands(const qint16 *stereo, int frames, int rate, float *bands, int count)
{
    std::vector<std::complex<float>> a(static_cast<size_t>(frames));
    for (int i = 0; i < frames; i++) {
        const float mono = (float(stereo[i * 2]) + float(stereo[i * 2 + 1])) / 65536.f;
        const float hann = 0.5f - 0.5f * float(std::cos(2 * kPi * i / (frames - 1)));
        a[size_t(i)] = mono * hann;
    }
    fft(a);
    // From 40 Hz to 16 kHz, each band the same step up in pitch.
    const double lowHz = 40, highHz = 16000;
    const double binHz = double(rate) / frames;
    for (int b = 0; b < count; b++) {
        const double f0 = lowHz * std::pow(highHz / lowHz, double(b) / count);
        const double f1 = lowHz * std::pow(highHz / lowHz, double(b + 1) / count);
        int k0 = int(std::floor(f0 / binHz)), k1 = int(std::ceil(f1 / binHz));
        k0 = std::clamp(k0, 1, frames / 2 - 1);
        k1 = std::clamp(k1, k0 + 1, frames / 2);
        float top = 0;
        for (int k = k0; k < k1; k++)
            top = std::max(top, std::abs(a[size_t(k)]));
        // A full-scale tone reads about a quarter of the frame size here.
        const float db = 20 * std::log10(std::max(top / (frames * 0.25f), 1e-5f));
        bands[b] = std::clamp((db + 60) / 60, 0.f, 1.f);
    }
}

FlashLimiter::FlashLimiter(float deadBand, float rise) : deadBand_(deadBand), rise_(rise), memory_(size_t(kTilesX * kTilesY), 0.f) {}

void FlashLimiter::setLimits(float deadBand, float rise)
{
    deadBand_ = deadBand;
    rise_ = rise;
}

void FlashLimiter::reset()
{
    std::fill(memory_.begin(), memory_.end(), 0.f);
}

float FlashLimiter::lightOf(int r, int g, int b)
{
    static const std::vector<float> linear = [] {
        std::vector<float> t(256);
        for (size_t i = 0; i < t.size(); i++)
            t[i] = float(std::pow(double(i) / 255, 2.2));
        return t;
    }();
    const float lr = linear[size_t(r)], lg = linear[size_t(g)], lb = linear[size_t(b)];
    const float light = 0.2126f * lr + 0.7152f * lg + 0.0722f * lb;
    const float sum = lr + lg + lb;
    const float red = sum > 0 && lr / sum >= 0.8f ? (lr - lg - lb) * 1.6f : 0.f;
    return std::max(light, red);
}

void FlashLimiter::apply(QImage &picture, double dt)
{
    const int w = picture.width(), h = picture.height();
    if (w < kTilesX || h < kTilesY)
        return;
    int scales[kTilesY][kTilesX];
    bool any = false;
    for (int ty = 0; ty < kTilesY; ty++) {
        const int y0 = ty * h / kTilesY, y1 = (ty + 1) * h / kTilesY;
        for (int tx = 0; tx < kTilesX; tx++) {
            const int x0 = tx * w / kTilesX, x1 = (tx + 1) * w / kTilesX;
            // A sample every few pixels is plenty to know how bright a tile is.
            float sum = 0;
            int count = 0;
            for (int y = y0; y < y1; y += 3) {
                const auto *line = reinterpret_cast<const QRgb *>(picture.constScanLine(y));
                for (int x = x0; x < x1; x += 3) {
                    sum += lightOf(qRed(line[x]), qGreen(line[x]), qBlue(line[x]));
                    count++;
                }
            }
            const float light = count ? sum / float(count) : 0;
            float &memory = memory_[size_t(ty * kTilesX + tx)];
            memory = std::min(light, memory + rise_ * float(dt));
            const float allowed = memory + deadBand_;
            // The sum was of light, the picture is in screen values.
            scales[ty][tx] = light <= allowed ? 256 : int(std::pow(double(allowed / light), 1 / 2.2) * 256);
            any = any || scales[ty][tx] < 256;
        }
    }
    if (!any)
        return;
    // Each corner takes the darkest of the tiles that meet there, and a tile blends between its
    // corners. So there are no visible edges, and no tile ends up brighter than it's allowed.
    int corners[kTilesY + 1][kTilesX + 1];
    for (int cy = 0; cy <= kTilesY; cy++) {
        for (int cx = 0; cx <= kTilesX; cx++) {
            int least = 256;
            for (int ty = std::max(cy - 1, 0); ty <= std::min(cy, kTilesY - 1); ty++)
                for (int tx = std::max(cx - 1, 0); tx <= std::min(cx, kTilesX - 1); tx++)
                    least = std::min(least, scales[ty][tx]);
            corners[cy][cx] = least;
        }
    }
    for (int ty = 0; ty < kTilesY; ty++) {
        const int y0 = ty * h / kTilesY, y1 = (ty + 1) * h / kTilesY;
        for (int tx = 0; tx < kTilesX; tx++) {
            const int a = corners[ty][tx], b = corners[ty][tx + 1], c = corners[ty + 1][tx], d = corners[ty + 1][tx + 1];
            if (a == 256 && b == 256 && c == 256 && d == 256)
                continue;
            const int x0 = tx * w / kTilesX, x1 = (tx + 1) * w / kTilesX;
            for (int y = y0; y < y1; y++) {
                auto *line = reinterpret_cast<QRgb *>(picture.scanLine(y));
                const int left = a + (c - a) * (y - y0) / (y1 - y0), right = b + (d - b) * (y - y0) / (y1 - y0);
                for (int x = x0; x < x1; x++) {
                    const int scale = left + (right - left) * (x - x0) / (x1 - x0);
                    const QRgb px = line[x];
                    line[x] = qRgb(qRed(px) * scale >> 8, qGreen(px) * scale >> 8, qBlue(px) * scale >> 8);
                }
            }
        }
    }
}

VisualizerView::VisualizerView(AudioOutput *audio, QWidget *parent)
    : QWidget(parent), audio_(audio), limiter_(wv::kGuard.deadBand, wv::kGuard.rise)
{
    setAccessibleName(QStringLiteral("Visualizer. Click for the next style."));
    setMinimumSize(200, 120);
    setCursor(Qt::PointingHandCursor);
    setAttribute(Qt::WA_OpaquePaintEvent);
    low_ = QColor(0x2f, 0xd6, 0xc0);
    high_ = QColor(0xff, 0xb4, 0x3c);
    bg_ = QColor(0x05, 0x08, 0x12);
    samples_.assign(kFrames * 2, 0);
    bands_.assign(kBands, 0);
    peaks_.assign(kBands, 0);
    fall_.assign(kBands, 0);
    timer_.setInterval(kFrameMs);
    connect(&timer_, &QTimer::timeout, this, &VisualizerView::step);
}

QStringList VisualizerView::styleNames()
{
    return {QStringLiteral("Bars"), QStringLiteral("Scope"), QStringLiteral("Rings"), QStringLiteral("Plasma")};
}

void VisualizerView::setCalm(bool on)
{
    calm_ = on;
    const wv::GuardLimits limits = on ? wv::kGuardCalm : wv::kGuard;
    limiter_.setLimits(limits.deadBand, limits.rise);
    if (on && style_ > Scope)
        setStyle(Bars);
}

void VisualizerView::setStyle(int style)
{
    style_ = (style % StyleCount + StyleCount) % StyleCount;
    if (calm_ && style_ > Scope)
        style_ = Bars;
    canvas_ = QImage();
    update();
    emit styleChanged(style_);
}

void VisualizerView::nextStyle()
{
    setStyle(style_ + 1);
}

void VisualizerView::setColors(const QColor &low, const QColor &high, const QColor &background)
{
    low_ = low;
    high_ = high;
    bg_ = background;
    canvas_ = QImage();
    update();
}

void VisualizerView::showEvent(QShowEvent *)
{
    if (audio_)
        audio_->setTapEnabled(true);
    limiter_.reset(); // it comes up gently each time
    clock_.start();
    timer_.start();
}

void VisualizerView::hideEvent(QHideEvent *)
{
    timer_.stop();
    if (audio_)
        audio_->setTapEnabled(false);
}

void VisualizerView::mouseReleaseEvent(QMouseEvent *e)
{
    if (e->button() == Qt::LeftButton)
        nextStyle();
}

void VisualizerView::step()
{
    const int got = audio_ ? audio_->tapRead(samples_.data(), kFrames) : 0;
    if (got < kFrames)
        std::fill(samples_.begin() + got * 2, samples_.end(), qint16(0));
    std::vector<float> now(kBands);
    spectrumBands(samples_.data(), kFrames, audio_ ? audio_->sampleRate() : 48000, now.data(), kBands);
    float sum = 0, bassNow = 0;
    for (int b = 0; b < kBands; b++) {
        // Up at once, down gently, as a meter does.
        bands_[size_t(b)] = std::max(now[size_t(b)], bands_[size_t(b)] - 0.035f);
        if (bands_[size_t(b)] >= peaks_[size_t(b)]) {
            peaks_[size_t(b)] = bands_[size_t(b)];
            fall_[size_t(b)] = 0;
        } else {
            fall_[size_t(b)] += 0.0016f;
            peaks_[size_t(b)] = std::max(0.f, peaks_[size_t(b)] - fall_[size_t(b)]);
        }
        sum += bands_[size_t(b)];
        if (b < 5)
            bassNow += bands_[size_t(b)] / 5;
    }
    level_ = sum / kBands;
    bass_ = std::max(bassNow, bass_ - 0.03f);
    phase_ += (0.02f + level_ * 0.08f) * (calm_ ? 0.4f : 1.f);
    const double dt = clock_.isValid() ? std::min(clock_.restart() / 1000.0, 0.25) : 1.0 / 30;
    if (style_ == Rings || style_ == Plasma) {
        if (style_ == Rings)
            stepRings();
        else
            stepPlasma();
        frame_ = canvas_; // the limiter's dimming mustn't feed back into the next picture
        frame_.detach();
    } else {
        const qreal ratio = devicePixelRatioF();
        const QSize want = size() * ratio;
        if (frame_.size() != want || frame_.format() != QImage::Format_RGB32) {
            frame_ = QImage(want, QImage::Format_RGB32);
            frame_.setDevicePixelRatio(ratio);
        }
        QPainter p(&frame_);
        p.fillRect(rect(), bg_);
        if (style_ == Bars)
            drawBars(p);
        else
            drawScope(p);
    }
    limiter_.apply(frame_, dt);
    update();
}

void VisualizerView::paintEvent(QPaintEvent *)
{
    QPainter p(this);
    p.fillRect(rect(), bg_);
    if (!frame_.isNull()) {
        p.setRenderHint(QPainter::SmoothPixmapTransform);
        p.drawImage(rect(), frame_);
    }
}

void VisualizerView::drawBars(QPainter &p)
{
    const int gap = 3;
    const double w = double(width() - gap * (kBands + 1)) / kBands;
    const int ledH = 5, ledGap = 2;
    const int rows = std::max(4, (height() - 12) / (ledH + ledGap));
    for (int b = 0; b < kBands; b++) {
        const double x = gap + b * (w + gap);
        const int lit = int(std::round(bands_[size_t(b)] * rows));
        for (int r = 0; r < lit; r++) {
            const float t = float(r) / float(std::max(1, rows - 1));
            p.fillRect(QRectF(x, height() - 6 - (r + 1) * (ledH + ledGap), w, ledH), mix(low_, high_, t));
        }
        const int peak = int(std::round(peaks_[size_t(b)] * rows));
        if (peak > 0)
            p.fillRect(QRectF(x, height() - 6 - peak * (ledH + ledGap), w, ledH), QColor(255, 255, 255, 220));
    }
}

void VisualizerView::drawScope(QPainter &p)
{
    p.setRenderHint(QPainter::Antialiasing);
    const auto trace = [&](int channel, double centre, const QColor &colour) {
        QPainterPath path;
        const int n = std::min(kFrames, width() * 2);
        for (int i = 0; i < n; i++) {
            const double x = double(i) * width() / n;
            const double y = centre - double(samples_[size_t(i * 2 + channel)]) / 32768.0 * height() * 0.42;
            if (i == 0)
                path.moveTo(x, y);
            else
                path.lineTo(x, y);
        }
        QColor glow = colour;
        glow.setAlpha(60);
        p.setPen(QPen(glow, 5));
        p.drawPath(path);
        p.setPen(QPen(colour, 1.5));
        p.drawPath(path);
    };
    p.setPen(QColor(255, 255, 255, 24));
    p.drawLine(0, height() / 3, width(), height() / 3);
    p.drawLine(0, height() * 2 / 3, width(), height() * 2 / 3);
    trace(0, height() / 3.0, low_);
    trace(1, height() * 2 / 3.0, high_);
}

// Each picture is the last one, a little larger and fainter, with the sound drawn over it.
void VisualizerView::stepRings()
{
    const int w = 320, h = 180;
    if (canvas_.isNull()) {
        canvas_ = QImage(w, h, QImage::Format_RGB32);
        canvas_.fill(bg_);
    }
    QImage last = canvas_;
    QPainter p(&canvas_);
    p.fillRect(canvas_.rect(), bg_);
    p.setRenderHint(QPainter::SmoothPixmapTransform);
    p.setOpacity(0.90);
    const double grow = 1.018 + bass_ * 0.03;
    p.translate(w / 2.0, h / 2.0);
    p.rotate(0.35 + double(level_) * 1.4);
    p.scale(grow, grow);
    p.drawImage(QPointF(-w / 2.0, -h / 2.0), last);
    p.resetTransform();
    p.setOpacity(1);
    p.setRenderHint(QPainter::Antialiasing);
    p.translate(w / 2.0, h / 2.0);
    const double base = 16 + bass_ * 22;
    for (int b = 0; b < kBands; b++) {
        const double a0 = 2 * kPi * b / kBands + phase_;
        const double r = base + bands_[size_t(b)] * 52;
        const QPointF inner(std::cos(a0) * base, std::sin(a0) * base);
        const QPointF outer(std::cos(a0) * r, std::sin(a0) * r);
        p.setPen(QPen(mix(low_, high_, float(b) / kBands), 2));
        p.drawLine(inner, outer);
    }
}

void VisualizerView::stepPlasma()
{
    const int w = 192, h = 108;
    if (canvas_.isNull())
        canvas_ = QImage(w, h, QImage::Format_RGB32);
    static const std::vector<float> table = [] {
        std::vector<float> t(1024);
        for (size_t i = 0; i < t.size(); i++)
            t[i] = float(std::sin(2 * kPi * double(i) / 1024));
        return t;
    }();
    const auto wave = [](float v) { return table[size_t(int(v * 162.97f)) & 1023]; };
    const float t = phase_ * 2.2f;
    const float energy = 0.25f + bass_ * 0.75f;
    for (int y = 0; y < h; y++) {
        auto *line = reinterpret_cast<QRgb *>(canvas_.scanLine(y));
        const float fy = float(y) * 0.075f;
        for (int x = 0; x < w; x++) {
            const float fx = float(x) * 0.06f;
            const float v = wave(fx + t) + wave(fy - t * 0.7f) + wave((fx + fy) * 0.6f + t * 0.5f) + wave(fx * 0.5f - fy * 0.8f + t * 1.3f);
            const float n = (v + 4) / 8;
            const float glow = energy * (0.35f + 0.65f * n);
            const float mixAt = 0.5f + 0.5f * wave(n * 6.28f + t);
            const float r = (low_.redF() + (high_.redF() - low_.redF()) * mixAt) * glow;
            const float g = (low_.greenF() + (high_.greenF() - low_.greenF()) * mixAt) * glow;
            const float b = (low_.blueF() + (high_.blueF() - low_.blueF()) * mixAt) * glow;
            line[x] = qRgb(int(std::min(r, 1.f) * 255), int(std::min(g, 1.f) * 255), int(std::min(b, 1.f) * 255));
        }
    }
}

} // namespace wmp
