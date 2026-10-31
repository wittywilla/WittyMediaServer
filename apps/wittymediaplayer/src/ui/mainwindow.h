// The player's window.
#pragma once

#include <QJsonArray>
#include <QMainWindow>
#include <QSettings>
#include <QTimer>
#include <QVector>
#include <memory>

class QAbstractButton;
class QAction;
class QFrame;
class QLabel;
class QMenu;
class QVBoxLayout;

namespace wv {
class VisualizerWidget;
}

namespace wmp {

class ControlBar;
class DiscController;
class EngineClient;
class MessageOverlay;
class Player;
class VideoView;

class MainWindow : public QMainWindow {
    Q_OBJECT
public:
    explicit MainWindow(bool noAudioDevice = false, QWidget *parent = nullptr);
    ~MainWindow() override;

    // Opens a file, a disc image, a disc folder or a drive.
    void openPath(const QString &path);

    Player *player() const { return player_; }
    DiscController *disc() const { return disc_; }
    VideoView *view() const { return view_; }
    wv::VisualizerWidget *visualizer() const { return viz_; }
    bool visualizerShowing() const;
    EngineClient *engine() const { return engine_; }

    // What's on show, for tests and the window's own status line.
    QString statusText() const { return status_; }
    QString shownMessage() const;
    QString shownSource() const { return currentPath_; }
    int menuButtonCount() const;

    // Handles a key as the window would. Returns false if it isn't one of its keys.
    bool handleKey(int key, Qt::KeyboardModifiers mods = Qt::NoModifier);

    void showMessage(const QString &text, bool stopsPlayback = false);

    // Which libraries play the next thing opened: the viewer's choice, from Settings.
    void setUseVlc(bool on);

protected:
    void closeEvent(QCloseEvent *e) override;
    void dragEnterEvent(QDragEnterEvent *e) override;
    void dropEvent(QDropEvent *e) override;
    bool eventFilter(QObject *o, QEvent *e) override;
    void resizeEvent(QResizeEvent *e) override;
    void mouseMoveEvent(QMouseEvent *e) override;
    bool nativeEvent(const QByteArray &type, void *message, qintptr *result) override;

private:
    void buildMenus();
    void buildStartPage();
    void refreshDrives();
    void rebuildTrackMenus();
    void rebuildDiscMenus();
    void rebuildRecent();
    void rememberRecent(const QString &path);
    void updateHitButtons();
    void layoutOverlays();
    void onTick();
    void togglePlay();
    void setFullscreen(bool on);
    void showControls();
    void openDialog();
    void openFolderDialog();
    void makeImage(const QString &driveOrImage = QString());
    void openSettings();
    void applyEngine();
    void lookForModuleUpdates();
    void stopPlayback(bool showStart = true);
    void setStatus(const QString &text);
    void setVolumePercent(int percent);
    void cycleAudio();
    // Music only: on a video it would hide the picture. Made the first time it's wanted.
    void setVisualizer(bool on);
    void cycleSubtitle();
    void updateFileSubtitles();

    Player *player_ = nullptr;
    VideoView *view_ = nullptr;
    wv::VisualizerWidget *viz_ = nullptr;
    QAction *actViz_ = nullptr;
    EngineClient *engine_ = nullptr;
    DiscController *disc_ = nullptr;
    ControlBar *bar_ = nullptr;
    QWidget *central_ = nullptr;
    QWidget *start_ = nullptr;
    QVBoxLayout *driveList_ = nullptr;
    QLabel *announcer_ = nullptr;
    MessageOverlay *message_ = nullptr;
    QLabel *audioCaption_ = nullptr;
    QLabel *engineCredit_ = nullptr;
    QVector<QAbstractButton *> hits_;
    QMenu *menuAudio_ = nullptr, *menuSubs_ = nullptr, *menuDisc_ = nullptr, *menuTitles_ = nullptr, *menuChapters_ = nullptr, *menuAngles_ = nullptr, *menuRecent_ = nullptr, *menuDrives_ = nullptr;
    QAction *actStop_ = nullptr, *actImage_ = nullptr, *actLocks_ = nullptr, *actForced_ = nullptr, *actFull_ = nullptr;
    QTimer tick_, hideControls_, driveWatch_;
    QSettings settings_;
    QString currentPath_;
    QString status_;
    QString pendingDrivePrompt_;
    bool isFile_ = false;
    bool fullscreen_ = false;
    bool muted_ = false;
    int volume_ = 100;
    bool ended_ = false;
    uint64_t subSig_ = 0;
    QVector<int> audioIds_, subIds_;
    QVector<bool> subExternal_;
};

} // namespace wmp
