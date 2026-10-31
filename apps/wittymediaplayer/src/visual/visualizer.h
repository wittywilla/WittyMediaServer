// Pictures that move with the sound, for the CD screen. It reads the player's audio tap and
// draws on the processor, small and scaled up, so it stays light on modest machines and needs
// no graphics card, and it stops altogether when it isn't on screen. WittyVisualizer's shader
// effects are the richer choice beside it, from the same tap.
//
// Flashing can harm people with photosensitive epilepsy, so every picture goes through
// FlashLimiter, with the same limits as WittyVisualizer's guard, before it's shown.
#pragma once

#include <QColor>
#include <QElapsedTimer>
#include <QImage>
#include <QStringList>
#include <QTimer>
#include <QWidget>
#include <vector>

namespace wmp {

class AudioOutput;

// The loudness of each of count bands, spaced as ears hear them, 0 to 1. frames should be
// a power of two.
void spectrumBands(const qint16 *stereo, int frames, int rate, float *bands, int count);

// The same rule as WittyVisualizer's guard (safety.ts), on the processor. Each tile of the
// picture remembers how dark it has lately been, and may show at most that plus a small dead
// band, while the memory rises slowly and falling is free. So a swing inside any third of a
// second stays under 0.1 of full light, which WCAG 2.3.1 doesn't count as a flash.
class FlashLimiter {
public:
    FlashLimiter(float deadBand, float rise);
    void setLimits(float deadBand, float rise);
    void reset();
    // Takes a Format_RGB32 picture, and dims the tiles that would flash.
    void apply(QImage &picture, double dt);
    // A colour's light as the rule counts it, with saturated red counted as it says.
    static float lightOf(int r, int g, int b);

private:
    float deadBand_, rise_;
    std::vector<float> memory_;
};

class VisualizerView : public QWidget {
    Q_OBJECT
public:
    enum Style { Bars, Scope, Rings, Plasma, StyleCount };

    explicit VisualizerView(AudioOutput *audio, QWidget *parent = nullptr);

    static QStringList styleNames();
    void setStyle(int style);
    int style() const { return style_; }
    void nextStyle();
    void setColors(const QColor &low, const QColor &high, const QColor &background);
    // Slower and softer, and the two gentle styles only.
    void setCalm(bool on);

    // How much it's showing, 0 to 1, for tests.
    float level() const { return level_; }
    // Reads the tap and draws one step, without waiting for the timer.
    void step();

signals:
    void styleChanged(int style);

protected:
    void paintEvent(QPaintEvent *) override;
    void showEvent(QShowEvent *) override;
    void hideEvent(QHideEvent *) override;
    void mouseReleaseEvent(QMouseEvent *e) override;

private:
    void drawBars(QPainter &p);
    void drawScope(QPainter &p);
    void stepRings();
    void stepPlasma();

    AudioOutput *audio_;
    QTimer timer_;
    int style_ = Bars;
    QColor low_, high_, bg_;
    std::vector<qint16> samples_;
    std::vector<float> bands_, peaks_, fall_;
    float level_ = 0;
    float bass_ = 0;
    float phase_ = 0;
    bool calm_ = false;
    QImage canvas_; // what Rings and Plasma build on
    QImage frame_;  // what's shown, once it's been through the limiter
    FlashLimiter limiter_;
    QElapsedTimer clock_;
};

} // namespace wmp
