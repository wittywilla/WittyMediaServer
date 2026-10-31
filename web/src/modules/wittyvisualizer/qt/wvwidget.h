// The visualizer as a part of a Qt window: the picture and its controls, with the settings
// and the notice before first use. session.ts and Stage.tsx for the desktop.
//
// An app makes one of these, gives it a SampleTap the audio thread writes to and a QSettings
// to keep choices in, shows it, and passes it keys with handleKey().
#pragma once

#include "wvanalysis.h"
#include "wvengine.h"
#include "wvrenderer.h"
#include "wvtap.h"

#include <QRhiWidget>
#include <QSettings>
#include <QWidget>
#include <chrono>

class QComboBox;
class QLabel;
class QToolButton;

namespace wv {

class VisualizerView;

class VisualizerWidget : public QWidget {
    Q_OBJECT
public:
    explicit VisualizerWidget(SampleTap *tap, QSettings *settings, QWidget *parent = nullptr);
    ~VisualizerWidget() override;

    // Handles a key as the visualizer would. False if it isn't one of its keys.
    bool handleKey(int key);
    // Asks first, the first time. False if the person said no.
    bool confirmFirstUse();
    const Settings &settings() const { return settings_; }
    void setControlsVisible(bool on);
    // For the app's status line and tests.
    QString effectId() const;
    VisualizerView *view() const { return view_; }

signals:
    void closeRequested();
    void fullscreenRequested();

protected:
    void showEvent(QShowEvent *e) override;
    void hideEvent(QHideEvent *e) override;

private:
    void openSettings();
    void refresh();
    void saveSoon();

    SampleTap *tap_;
    QSettings *store_;
    Settings settings_;
    VisualizerView *view_ = nullptr;
    QWidget *bar_ = nullptr;
    QComboBox *effects_ = nullptr;
    QToolButton *star_ = nullptr;
    QLabel *stats_ = nullptr;
};

// The picture. It draws only when asked by the window system, and asks for the next frame
// only while it's on screen, so it stops by itself when hidden, minimised or closed.
class VisualizerView : public QRhiWidget {
    Q_OBJECT
public:
    VisualizerView(SampleTap *tap, Settings *settings, QWidget *parent = nullptr);

    void next();
    void previous();
    void select(const QString &id);
    QString current() const { return toId_.isEmpty() ? fromId_ : toId_; }
    void settingsChanged();
    void setRunning(bool on);

    double fps() const { return fps_; }
    double frameMs() const { return frameMs_; }
    QSize drawSize() const { return renderer_.size(); }
    double scale() const { return budget_.scale; }
    int beats() const { return analyzer_.frame().beatCount; }
    bool hearing() const { return hearing_; }
    QString problem() const { return problem_; }

signals:
    void effectChanged(const QString &id);
    void statsChanged();

protected:
    void initialize(QRhiCommandBuffer *cb) override;
    void render(QRhiCommandBuffer *cb) override;
    void releaseResources() override;

private:
    QStringList inRunning() const;
    void change(const QString &id);
    void finishMix();
    bool calm() const;
    void resetBudget();
    void readAudio(double dt);

    SampleTap *tap_;
    Settings *settings_;
    Renderer renderer_;
    Analyzer analyzer_;
    Cycle cycle_;
    Palettes palettes_;
    Budget budget_{1, 0.25, 1};
    std::vector<float> left_, right_;
    QString fromId_, toId_;
    double mix_ = 0, mixFor_ = 2.4, shownFor_ = 0;
    int mixStyle_ = 0;
    double time_ = 0, flow_ = 0, patternAt_ = 0;
    bool hearing_ = true, everHeard_ = false, running_ = false, ready_ = false, software_ = false;
    int quietFrames_ = 0;
    std::chrono::steady_clock::time_point last_;
    double fps_ = 0, frameMs_ = 0, costSum_ = 0, statsFor_ = 0;
    int statsFrames_ = 0;
    QString problem_;
};

} // namespace wv
