// Pictures that move with the sound. It reads the player's audio tap (AudioOutput::tapRead)
// and draws on the processor, small and scaled up, so it stays light on modest machines,
// and it stops altogether when it isn't on screen. Another visualizer can take its place in
// the now-playing area: all it needs is that tap.
#pragma once

#include <QColor>
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
    QImage canvas_;
};

} // namespace wmp
