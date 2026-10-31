// The CD screen: a tracklist with checkboxes, a big now-playing area and the controls for
// copying a disc to music files or to an image. It has its own glossy, early-desktop look,
// all drawn here, and can follow the app's normal theme instead.
#pragma once

#include "cdripper.h"

#include <QImage>
#include <QJsonObject>
#include <QMap>
#include <QPointer>
#include <QTimer>
#include <QWidget>

class QAbstractButton;
class QCheckBox;
class QComboBox;
class QHBoxLayout;
class QLabel;
class QLineEdit;
class QProcess;
class QProgressBar;
class QPushButton;
class QSettings;
class QSpinBox;
class QTreeWidget;
class QTreeWidgetItem;
class QVBoxLayout;

namespace wmp {

class CoverView;
class GelButton;
class LcdReadout;
class Player;
class SeekBar;
class VisualizerView;
class VolumeBar;

class CdPage : public QWidget {
    Q_OBJECT
public:
    CdPage(Player *player, QSettings *settings, QWidget *parent = nullptr);
    ~CdPage() override;

    void setDisc(const CdInfo &info);
    const CdInfo &disc() const { return info_; }
    // Stops playing and copying, and forgets the disc.
    void closeDisc();

    // A key the screen wants, given what has focus. True when it dealt with it.
    bool handleKey(int key, QWidget *focus);

    void setClassicLook(bool on);
    bool classicLook() const { return classic_; }
    void setVolumePercent(int percent);

    // Another visualizer can stand where the built-in one does.
    void setVisualizer(QWidget *visualizer);
    VisualizerView *visualizer() const { return visual_; }

    void playTrack(int index);
    void togglePlay();
    void stopPlaying();
    void previous();
    void next();
    int currentTrack() const { return current_; }
    bool playing() const { return playing_; }
    QString statusText() const;

    void setChecked(const QVector<int> &numbers);
    QVector<int> checkedTracks() const;
    void setRipOptions(const QString &format, const QString &quality, const QString &folder, const QString &nameTemplate, bool secure, int offset);
    void startRip();
    void stopRip();
    bool busy() const;
    QVector<RipResult> results() const { return results_; }
    QString lastReport() const { return lastReport_; }
    QStringList qualitiesOf(const QString &format) const;

    void makeImage(const QString &cuePath);
    void lookUp();
    // Names from the lookup, as the updater gives them.
    void applyMatch(const QJsonObject &match);
    void setCover(const QByteArray &bytes);

signals:
    void backRequested();
    void volumeChanged(int percent);
    void statusChanged(const QString &text);
    void ripFinished();

protected:
    void paintEvent(QPaintEvent *) override;

private:
    void build();
    void applyLook();
    void fillTracks();
    void tick();
    void showTrack();
    void setStatus(const QString &text);
    void setBusyControls();
    void refillQualities();
    void chooseFolder();
    void chooseImage();
    void saveOptions();
    QTreeWidgetItem *itemFor(int number) const;
    void markTrack(int number, int percent, const QString &text);
    RipJob buildJob() const;
    void fetchCover(const QString &releaseId);
    QString updaterPath() const;

    Player *player_;
    QSettings *settings_;
    CdInfo info_;
    CdRipper *ripper_;
    QVector<RipResult> results_;
    QString lastReport_;
    QProcess *imageProc_ = nullptr;
    QPointer<QProcess> lookup_;
    QByteArray cover_;
    QTimer tick_;

    bool classic_ = true;
    bool streamOpen_ = false;
    bool playing_ = false;
    bool userPaused_ = false;
    int current_ = 0;
    double pendingSeek_ = -1;
    double seekTarget_ = -1; // the display holds its track until the sound gets here
    int seekWait_ = 0;
    bool showRemaining_ = false;

    QVBoxLayout *visualHolder_ = nullptr;
    QWidget *visualWidget_ = nullptr;
    VisualizerView *visual_ = nullptr;
    CoverView *coverView_ = nullptr;
    LcdReadout *lcd_ = nullptr;
    QLabel *headTitle_ = nullptr, *nowTitle_ = nullptr, *nowArtist_ = nullptr, *statusLabel_ = nullptr, *visualName_ = nullptr, *offsetLabel_ = nullptr;
    SeekBar *seek_ = nullptr;
    VolumeBar *volume_ = nullptr;
    GelButton *playBtn_ = nullptr, *stopBtn_ = nullptr, *prevBtn_ = nullptr, *nextBtn_ = nullptr;
    QTreeWidget *tree_ = nullptr;
    QLineEdit *album_ = nullptr, *artist_ = nullptr, *year_ = nullptr, *folder_ = nullptr, *template_ = nullptr;
    QComboBox *format_ = nullptr, *quality_ = nullptr, *look_ = nullptr;
    QCheckBox *secure_ = nullptr;
    QSpinBox *offset_ = nullptr;
    QPushButton *chooseBtn_ = nullptr, *ripBtn_ = nullptr, *imageBtn_ = nullptr, *lookupBtn_ = nullptr, *stopRipBtn_ = nullptr, *visualBtn_ = nullptr, *backBtn_ = nullptr;
    QProgressBar *overall_ = nullptr;
    int ripTotal_ = 0, ripDone_ = 0;
    int ripPercent_ = 0;
};

} // namespace wmp
