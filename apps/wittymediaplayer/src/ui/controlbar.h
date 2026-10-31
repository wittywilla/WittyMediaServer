// The strip of controls under the picture.
#pragma once

#include <QSlider>
#include <QToolButton>
#include <QVector>
#include <QWidget>

class QLabel;

namespace wmp {

// The time line, with chapter marks. A keyboard moves it in ten-second steps, and a
// screen reader hears the time.
class SeekBar : public QSlider {
    Q_OBJECT
public:
    explicit SeekBar(QWidget *parent = nullptr);
    void setDuration(double seconds);
    void setPosition(double seconds);
    void setChapters(const QVector<double> &chapters);
    double duration() const { return duration_; }
    double valueSeconds() const { return value() / 1000.0; }
    bool dragging() const { return isSliderDown(); }

signals:
    void seekRequested(double seconds);

protected:
    void paintEvent(QPaintEvent *) override;
    void mousePressEvent(QMouseEvent *e) override;
    void mouseMoveEvent(QMouseEvent *e) override;
    void mouseReleaseEvent(QMouseEvent *e) override;
    void keyPressEvent(QKeyEvent *e) override;

private:
    double secondsAt(int x) const;
    double duration_ = 0;
    QVector<double> chapters_;
    bool down_ = false;
};

// The volume, drawn to match the time line.
class VolumeBar : public QSlider {
    Q_OBJECT
public:
    explicit VolumeBar(QWidget *parent = nullptr);

protected:
    void paintEvent(QPaintEvent *) override;
    void mousePressEvent(QMouseEvent *e) override;
    void mouseMoveEvent(QMouseEvent *e) override;
};

class ControlBar : public QWidget {
    Q_OBJECT
public:
    explicit ControlBar(QWidget *parent = nullptr);

    void setPlaying(bool playing);
    void setPosition(double pos, double dur);
    void setChapters(const QVector<double> &chapters);
    void setVolume(int percent, bool muted);
    void setFullscreen(bool on);
    void setDiscMode(bool disc);
    void setSeekable(bool on);
    void setHasMedia(bool on);
    void setHasTracks(bool audio, bool subs);

    SeekBar *seekBar() const { return seek_; }
    QToolButton *audioButton() const { return audio_; }
    QToolButton *subtitleButton() const { return subs_; }

    static QString formatTime(double seconds);

signals:
    void playPause();
    void previous();
    void next();
    void seekTo(double seconds);
    void volumeChanged(int percent);
    void muteToggled();
    void rootMenu();
    void titleMenu();
    void fullscreen();

private:
    QToolButton *tool(const QString &icon, const QString &name, const QString &tip);

    QToolButton *play_, *prev_, *next_, *mute_, *audio_, *subs_, *menu_, *title_, *full_;
    SeekBar *seek_;
    QSlider *volume_;
    QLabel *time_, *total_;
    bool playing_ = false;
};

} // namespace wmp
