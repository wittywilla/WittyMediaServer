#include "mainwindow.h"

#include "../disc/disccontroller.h"
#include "../engine/engineclient.h"
#include "../media/audiooutput.h"
#include "../media/player.h"
#include "../video/videoview.h"
#include "controlbar.h"
#include "imagedialog.h"
#include "subtitlepaint.h"
#include "theme.h"

#include <QAbstractButton>
#include <QAccessible>
#include <QActionGroup>
#include <QApplication>
#include <QCloseEvent>
#include <QDir>
#include <QDragEnterEvent>
#include <QFileDialog>
#include <QFileInfo>
#include <QHBoxLayout>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QKeyEvent>
#include <QLabel>
#include <QLocale>
#include <QMenu>
#include <QMenuBar>
#include <QMessageBox>
#include <QMimeData>
#include <QPainter>
#include <QPushButton>
#include <QStyle>
#include <QThread>
#include <QVBoxLayout>
#include <cmath>

#ifdef Q_OS_WIN
#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#include <dbt.h>
#endif

namespace wmp {

namespace {

// A disc's button: it shows nothing itself, since the disc's own artwork is its
// label, but a keyboard can reach it and a screen reader can name it.
class HitButton : public QAbstractButton {
public:
    explicit HitButton(QWidget *parent) : QAbstractButton(parent)
    {
        setFocusPolicy(Qt::TabFocus);
        setAttribute(Qt::WA_Hover);
        setAttribute(Qt::WA_TranslucentBackground);
        setCursor(Qt::PointingHandCursor);
    }
    bool selected = false;
    bool drawOutline = false;

protected:
    void paintEvent(QPaintEvent *) override
    {
        // Discs whose highlight isn't visible still need to show where the choice is.
        if (!drawOutline && !hasFocus())
            return;
        if (!selected && !hasFocus())
            return;
        QPainter p(this);
        p.setRenderHint(QPainter::Antialiasing);
        p.setPen(QPen(Themes::current().focus, 2));
        p.drawRoundedRect(rect().adjusted(1, 1, -1, -1), 4, 4);
    }
};

QString trackLabel(const QString &language, const QString &detail)
{
    const QString lang = languageName(language);
    return detail.isEmpty() ? lang : QStringLiteral("%1 (%2)").arg(lang, detail);
}

QString channelsText(int n)
{
    if (n <= 0)
        return {};
    if (n == 1)
        return QStringLiteral("Mono");
    if (n == 2)
        return QStringLiteral("Stereo");
    return QStringLiteral("%1.1").arg(n - 1);
}

} // namespace

// A plain message over the picture, with a button to dismiss it.
class MessageOverlay : public QWidget {
public:
    explicit MessageOverlay(QWidget *parent) : QWidget(parent)
    {
        setObjectName(QStringLiteral("messageBox"));
        setAttribute(Qt::WA_StyledBackground);
        auto *col = new QVBoxLayout(this);
        col->setContentsMargins(24, 20, 24, 20);
        col->setSpacing(14);
        label = new QLabel(this);
        label->setProperty("role", QStringLiteral("message"));
        label->setWordWrap(true);
        label->setAlignment(Qt::AlignCenter);
        label->setAccessibleName(QStringLiteral("Message"));
        ok = new QPushButton(QStringLiteral("OK"), this);
        ok->setProperty("role", QStringLiteral("primary"));
        col->addWidget(label);
        col->addWidget(ok, 0, Qt::AlignCenter);
        connect(ok, &QPushButton::clicked, this, [this] { hide(); });
        hide();
    }
    void say(const QString &text)
    {
        label->setText(text);
        adjustSize();
        show();
        raise();
        ok->setFocus();
        QAccessibleEvent ev(label, QAccessible::Alert);
        QAccessible::updateAccessibility(&ev);
    }
    QLabel *label;
    QPushButton *ok;
};

MainWindow::MainWindow(bool noAudioDevice, QWidget *parent)
    : QMainWindow(parent), settings_(QSettings::IniFormat, QSettings::UserScope, QStringLiteral("WittyMediaPlayer"), QStringLiteral("WittyMediaPlayer"))
{
    setWindowTitle(QStringLiteral("WittyMediaPlayer"));
    setAcceptDrops(true);
    resize(1120, 720);
    if (settings_.contains(QStringLiteral("window/geometry")))
        restoreGeometry(settings_.value(QStringLiteral("window/geometry")).toByteArray());

    Themes::apply(qApp, Themes::load(settings_.value(QStringLiteral("theme"), QStringLiteral("dark")).toString()));

    player_ = new Player(this);
    player_->initAudio(noAudioDevice);
    engine_ = new EngineClient(this);

    central_ = new QWidget(this);
    central_->setObjectName(QStringLiteral("central"));
    setCentralWidget(central_);
    view_ = new VideoView(central_);
    player_->setVideoSink(view_);
    bar_ = new ControlBar(central_);
    disc_ = new DiscController(engine_, player_, view_, this);

    announcer_ = new QLabel(central_);
    announcer_->setFixedSize(1, 1);
    announcer_->setAccessibleName(QStringLiteral("Status"));
    announcer_->move(-10, -10);

    audioCaption_ = new QLabel(view_);
    audioCaption_->setProperty("role", QStringLiteral("heading"));
    audioCaption_->setAlignment(Qt::AlignCenter);
    audioCaption_->hide();

    message_ = new MessageOverlay(central_);

    buildStartPage();
    buildMenus();

    volume_ = settings_.value(QStringLiteral("volume"), 100).toInt();
    muted_ = settings_.value(QStringLiteral("muted"), false).toBool();
    setVolumePercent(volume_);
    disc_->setObeyLocks(settings_.value(QStringLiteral("obeyLocks"), true).toBool());
    disc_->setShowForced(settings_.value(QStringLiteral("forcedSubs"), true).toBool());

    connect(bar_, &ControlBar::playPause, this, &MainWindow::togglePlay);
    connect(bar_, &ControlBar::previous, this, [this] { handleKey(Qt::Key_P); });
    connect(bar_, &ControlBar::next, this, [this] { handleKey(Qt::Key_N); });
    connect(bar_, &ControlBar::seekTo, this, [this](double sec) {
        if (disc_->active())
            disc_->seekTo(sec);
        else
            player_->seek(sec);
    });
    connect(bar_, &ControlBar::volumeChanged, this, &MainWindow::setVolumePercent);
    connect(bar_, &ControlBar::muteToggled, this, [this] {
        muted_ = !muted_;
        setVolumePercent(volume_);
    });
    connect(bar_, &ControlBar::rootMenu, disc_, &DiscController::rootMenu);
    connect(bar_, &ControlBar::titleMenu, disc_, &DiscController::titleMenu);
    connect(bar_, &ControlBar::fullscreen, this, [this] { setFullscreen(!fullscreen_); });

    connect(player_, &Player::failed, this, [this](const QString &m) { showMessage(m, !disc_->active()); });
    connect(player_, &Player::discTrouble, this, [this](const QString &m) { setStatus(m); });
    connect(player_, &Player::discLost, this, [this] {
        stopPlayback();
        showMessage(QStringLiteral("The disc was taken out of the drive."));
    });
    connect(player_, &Player::tracksChanged, this, &MainWindow::rebuildTrackMenus);
    connect(player_, &Player::opened, this, [this] {
        if (isFile_ && player_->isAudioOnly()) {
            const QImage art = player_->coverArt();
            QVector<Overlay> over;
            view_->clearPicture();
            if (!art.isNull())
                over.push_back({art.convertToFormat(QImage::Format_RGBA8888_Premultiplied), QRectF(0.25, 0.1, 0.5, 0.8), 1});
            view_->setOverlays(over);
            view_->setAspect(QStringLiteral("16:9"));
            const QString t = player_->title();
            audioCaption_->setText(t.isEmpty() ? QFileInfo(currentPath_).completeBaseName() : t);
            audioCaption_->setVisible(art.isNull());
            layoutOverlays();
        } else {
            audioCaption_->hide();
        }
    });
    connect(player_, &Player::ended, this, [this] {
        if (!disc_->active()) {
            ended_ = true;
            player_->setPaused(true);
            bar_->setPlaying(false);
            setStatus(QStringLiteral("Finished"));
        }
    });

    connect(disc_, &DiscController::changed, this, [this] {
        rebuildDiscMenus();
        rebuildTrackMenus();
        bar_->setSeekable(disc_->inTitle() && !disc_->locked(QStringLiteral("seek")));
        bar_->setChapters(disc_->chapters());
        if (disc_->active()) {
            const QString kind = disc_->presentationKind();
            if (kind == QLatin1String("title"))
                setStatus(QStringLiteral("Playing title %1 of %2").arg(disc_->ttn()).arg(disc_->label()));
            else if (kind == QLatin1String("menu"))
                setStatus(disc_->inMenu() ? QStringLiteral("Disc menu, %1 options. Use the arrow keys and Enter.").arg(disc_->buttons().size()) : QStringLiteral("Playing %1").arg(disc_->label()));
        }
    });
    connect(disc_, &DiscController::buttonsChanged, this, &MainWindow::updateHitButtons);
    connect(view_, &VideoView::pictureRectChanged, this, &MainWindow::updateHitButtons);
    connect(disc_, &DiscController::message, this, [this](const QString &m) { setStatus(m); });
    connect(disc_, &DiscController::stopped, this, [this](const QString &reason, bool isProtected) {
        if (isProtected) {
            stopPlayback();
            showMessage(reason);
            return;
        }
        bar_->setPlaying(false);
        // The disc has come to the end of its program: offer it from the top.
        if (!reason.isEmpty())
            setStatus(reason);
        stopPlayback();
        if (!reason.isEmpty())
            showMessage(reason);
    });
    connect(disc_, &DiscController::rememberAudio, this, [this](const QString &lang) {
        settings_.setValue(QStringLiteral("audioLanguage"), lang);
    });
    connect(disc_, &DiscController::rememberSubtitle, this, [this](const QString &lang, bool off) {
        settings_.setValue(QStringLiteral("subtitleLanguage"), off ? QString() : lang);
    });
    connect(engine_, &EngineClient::helperLost, this, [this] {
        if (disc_->active()) {
            stopPlayback();
            showMessage(QStringLiteral("WittyMediaPlayer's disc helper stopped unexpectedly. This disc may be damaged."));
        }
    });

    tick_.setInterval(200);
    connect(&tick_, &QTimer::timeout, this, &MainWindow::onTick);
    tick_.start();
    hideControls_.setSingleShot(true);
    hideControls_.setInterval(3500);
    connect(&hideControls_, &QTimer::timeout, this, [this] {
        if (fullscreen_ && !bar_->underMouse() && !bar_->isAncestorOf(QApplication::focusWidget())) {
            bar_->hide();
            setCursor(Qt::BlankCursor);
        }
    });

    qApp->installEventFilter(this);
    central_->setMouseTracking(true);
    view_->setMouseTracking(true);
    setMouseTracking(true);
    view_->setFocus();
    refreshDrives();
    start_->show();
    layoutOverlays();
}

MainWindow::~MainWindow()
{
    qApp->removeEventFilter(this);
}

void MainWindow::closeEvent(QCloseEvent *e)
{
    settings_.setValue(QStringLiteral("window/geometry"), saveGeometry());
    settings_.setValue(QStringLiteral("volume"), volume_);
    settings_.setValue(QStringLiteral("muted"), muted_);
    player_->stop();
    engine_->stop();
    QMainWindow::closeEvent(e);
}

// ---------------------------------------------------------------------- menus

void MainWindow::buildMenus()
{
    QMenu *file = menuBar()->addMenu(QStringLiteral("&File"));
    file->addAction(Themes::icon(QStringLiteral("open")), QStringLiteral("&Open file…"), QKeySequence::Open, this, &MainWindow::openDialog);
    file->addAction(Themes::icon(QStringLiteral("open")), QStringLiteral("Open disc &folder…"), QKeySequence(QStringLiteral("Ctrl+Shift+O")), this, &MainWindow::openFolderDialog);
    menuDrives_ = file->addMenu(Themes::icon(QStringLiteral("disc")), QStringLiteral("Play from &drive"));
    connect(menuDrives_, &QMenu::aboutToShow, this, &MainWindow::refreshDrives);
    menuRecent_ = file->addMenu(QStringLiteral("&Recent"));
    file->addSeparator();
    actImage_ = file->addAction(QStringLiteral("Make disc &image…"), QKeySequence(QStringLiteral("Ctrl+I")), this, [this] { makeImage(); });
    file->addSeparator();
    actStop_ = file->addAction(QStringLiteral("&Close"), QKeySequence(QStringLiteral("Ctrl+W")), this, [this] { stopPlayback(); });
    file->addAction(QStringLiteral("&Quit"), QKeySequence::Quit, this, &QWidget::close);

    QMenu *play = menuBar()->addMenu(QStringLiteral("&Playback"));
    play->addAction(QStringLiteral("Play or pause\tSpace"), this, &MainWindow::togglePlay);
    play->addAction(QStringLiteral("Back 10 seconds\tLeft"), this, [this] { handleKey(Qt::Key_Left); });
    play->addAction(QStringLiteral("Forward 10 seconds\tRight"), this, [this] { handleKey(Qt::Key_Right); });
    play->addAction(QStringLiteral("Previous chapter\tP"), this, [this] { handleKey(Qt::Key_P); });
    play->addAction(QStringLiteral("Next chapter\tN"), this, [this] { handleKey(Qt::Key_N); });
    play->addSeparator();
    play->addAction(QStringLiteral("Volume up\tUp"), this, [this] { setVolumePercent(qMin(100, volume_ + 5)); });
    play->addAction(QStringLiteral("Volume down\tDown"), this, [this] { setVolumePercent(qMax(0, volume_ - 5)); });

    menuAudio_ = menuBar()->addMenu(QStringLiteral("&Audio"));
    menuSubs_ = menuBar()->addMenu(QStringLiteral("&Subtitles"));
    bar_->audioButton()->setMenu(menuAudio_);
    bar_->subtitleButton()->setMenu(menuSubs_);

    menuDisc_ = menuBar()->addMenu(QStringLiteral("&Disc"));
    menuDisc_->addAction(QStringLiteral("Disc menu\tM"), disc_, &DiscController::rootMenu);
    menuDisc_->addAction(QStringLiteral("Title menu\tT"), disc_, &DiscController::titleMenu);
    menuDisc_->addAction(QStringLiteral("Resume"), disc_, &DiscController::resume);
    menuDisc_->addAction(QStringLiteral("Play main feature"), disc_, &DiscController::playMain);
    menuDisc_->addAction(QStringLiteral("Skip this clip"), disc_, &DiscController::skip);
    menuTitles_ = menuDisc_->addMenu(QStringLiteral("Titles"));
    menuChapters_ = menuDisc_->addMenu(QStringLiteral("Chapters"));
    menuAngles_ = menuDisc_->addMenu(QStringLiteral("Camera angle"));
    menuDisc_->addSeparator();
    actLocks_ = menuDisc_->addAction(QStringLiteral("Obey the disc's restrictions"));
    actLocks_->setCheckable(true);
    actLocks_->setChecked(settings_.value(QStringLiteral("obeyLocks"), true).toBool());
    actLocks_->setToolTip(QStringLiteral("Some discs forbid skipping warnings or changing tracks at certain points, as a disc player would."));
    connect(actLocks_, &QAction::toggled, this, [this](bool on) {
        disc_->setObeyLocks(on);
        settings_.setValue(QStringLiteral("obeyLocks"), on);
        bar_->setSeekable(disc_->inTitle() && !disc_->locked(QStringLiteral("seek")));
    });
    actForced_ = menuDisc_->addAction(QStringLiteral("Show forced captions when subtitles are off"));
    actForced_->setCheckable(true);
    actForced_->setChecked(settings_.value(QStringLiteral("forcedSubs"), true).toBool());
    connect(actForced_, &QAction::toggled, this, [this](bool on) {
        disc_->setShowForced(on);
        settings_.setValue(QStringLiteral("forcedSubs"), on);
    });

    QMenu *view = menuBar()->addMenu(QStringLiteral("&View"));
    actFull_ = view->addAction(QStringLiteral("Full screen\tF"), this, [this] { setFullscreen(!fullscreen_); });
    QMenu *themes = view->addMenu(QStringLiteral("Theme"));
    auto *group = new QActionGroup(this);
    const QString currentTheme = Themes::current().id;
    for (const QString &id : Themes::ids()) {
        const Theme t = Themes::load(id);
        QAction *a = themes->addAction(t.name);
        a->setCheckable(true);
        a->setChecked(id == currentTheme);
        group->addAction(a);
        connect(a, &QAction::triggered, this, [this, id] {
            Themes::apply(qApp, Themes::load(id));
            settings_.setValue(QStringLiteral("theme"), id);
            update();
        });
    }

    QMenu *help = menuBar()->addMenu(QStringLiteral("&Help"));
    help->addAction(QStringLiteral("Keyboard shortcuts"), this, [this] {
        QMessageBox::information(this, QStringLiteral("Keyboard shortcuts"),
            QStringLiteral("Space or K: play or pause\nLeft and Right: back and forward 10 seconds\nUp and Down: volume\n"
                           "F: full screen, Esc: leave it\nA: next audio track\nS: next subtitle track\nN and P: next and previous chapter\n"
                           "Number keys: go to a chapter\n\nOn a disc:\nArrow keys and Enter: choose in the disc's menu\nM: disc menu\nT: title menu"));
    });
    help->addAction(QStringLiteral("About WittyMediaPlayer"), this, [this] {
        QMessageBox::about(this, QStringLiteral("About WittyMediaPlayer"),
            QStringLiteral("<b>WittyMediaPlayer %1</b><br>Plays video, audio, DVDs and Blu-rays on this computer. "
                           "Nothing leaves it, and it collects nothing.<br><br>"
                           "WittyMediaPlayer, developed by WittyWilla. Apache License 2.0.<br>"
                           "Uses WittyDVD, WittyBluray and WittyDisc, developed by WittyWilla.<br>"
                           "Uses Qt and FFmpeg under the LGPL: they're separate libraries in its folder, which you can replace. "
                           "See the licenses folder.")
                .arg(QCoreApplication::applicationVersion()));
    });
    rebuildRecent();
    rebuildDiscMenus();
    rebuildTrackMenus();
}

void MainWindow::buildStartPage()
{
    start_ = new QWidget(central_);
    start_->setObjectName(QStringLiteral("startPage"));
    start_->setAttribute(Qt::WA_StyledBackground);
    auto *outer = new QVBoxLayout(start_);
    outer->setContentsMargins(40, 40, 40, 40);
    outer->addStretch(2);
    auto *brand = new QLabel(QStringLiteral("WittyMediaPlayer"), start_);
    brand->setProperty("role", QStringLiteral("brand"));
    brand->setAlignment(Qt::AlignCenter);
    outer->addWidget(brand);
    auto *tag = new QLabel(QStringLiteral("Video, audio, DVDs and Blu-rays, on this computer."), start_);
    tag->setProperty("role", QStringLiteral("muted"));
    tag->setAlignment(Qt::AlignCenter);
    outer->addWidget(tag);
    outer->addSpacing(24);
    auto *row = new QHBoxLayout;
    row->addStretch(1);
    auto *open = new QPushButton(QStringLiteral("Open a file"), start_);
    open->setProperty("role", QStringLiteral("primary"));
    connect(open, &QPushButton::clicked, this, &MainWindow::openDialog);
    auto *folder = new QPushButton(QStringLiteral("Open a disc folder"), start_);
    connect(folder, &QPushButton::clicked, this, &MainWindow::openFolderDialog);
    row->addWidget(open);
    row->addWidget(folder);
    row->addStretch(1);
    outer->addLayout(row);
    outer->addSpacing(8);
    auto *hint = new QLabel(QStringLiteral("Or drop a file, a disc image or a disc folder on this window."), start_);
    hint->setProperty("role", QStringLiteral("hint"));
    hint->setAlignment(Qt::AlignCenter);
    outer->addWidget(hint);
    outer->addSpacing(20);
    auto *drives = new QWidget(start_);
    driveList_ = new QVBoxLayout(drives);
    driveList_->setContentsMargins(0, 0, 0, 0);
    driveList_->setSpacing(8);
    drives->setMaximumWidth(560);
    outer->addWidget(drives, 0, Qt::AlignHCenter);
    outer->addStretch(3);
}

void MainWindow::refreshDrives()
{
    engine_->call(QStringLiteral("drives"), {}, [this](const EngineReply &r) {
        if (!r.ok())
            return;
        const QJsonArray list = r.result.toArray();
        menuDrives_->clear();
        while (QLayoutItem *it = driveList_->takeAt(0)) {
            delete it->widget();
            delete it;
        }
        if (list.isEmpty()) {
            auto *none = menuDrives_->addAction(QStringLiteral("No disc drive found"));
            none->setEnabled(false);
        }
        for (const auto &v : list) {
            const QJsonObject d = v.toObject();
            const QString path = d.value("path").toString(), name = d.value("name").toString(), label = d.value("label").toString();
            const bool media = d.value("media").toBool(), audio = d.value("audio").toBool();
            const QString what = !media ? QStringLiteral("no disc") : audio ? QStringLiteral("audio CD") : (label.isEmpty() ? QStringLiteral("disc") : label);
            QAction *a = menuDrives_->addAction(QStringLiteral("%1 (%2)").arg(name, what));
            a->setEnabled(media && !audio);
            connect(a, &QAction::triggered, this, [this, path] { openPath(path); });

            auto *rowW = new QWidget;
            rowW->setObjectName(QStringLiteral("driveRow"));
            rowW->setAttribute(Qt::WA_StyledBackground);
            auto *rowL = new QHBoxLayout(rowW);
            rowL->setContentsMargins(14, 10, 10, 10);
            auto *caption = new QLabel(QStringLiteral("%1  %2").arg(name, what), rowW);
            auto *playBtn = new QPushButton(QStringLiteral("Play"), rowW);
            playBtn->setAccessibleName(QStringLiteral("Play the disc in %1").arg(name));
            playBtn->setEnabled(media && !audio);
            connect(playBtn, &QPushButton::clicked, this, [this, path] { openPath(path); });
            auto *imgBtn = new QPushButton(QStringLiteral("Make image"), rowW);
            imgBtn->setAccessibleName(QStringLiteral("Make a disc image from %1").arg(name));
            imgBtn->setEnabled(media && !audio);
            connect(imgBtn, &QPushButton::clicked, this, [this, path] { makeImage(path); });
            rowL->addWidget(caption, 1);
            rowL->addWidget(playBtn);
            rowL->addWidget(imgBtn);
            driveList_->addWidget(rowW);
        }
    });
}

void MainWindow::rebuildRecent()
{
    menuRecent_->clear();
    const QStringList recent = settings_.value(QStringLiteral("recent")).toStringList();
    if (recent.isEmpty()) {
        menuRecent_->addAction(QStringLiteral("Nothing yet"))->setEnabled(false);
        return;
    }
    for (const QString &p : recent) {
        QAction *a = menuRecent_->addAction(QFileInfo(p).fileName().isEmpty() ? p : QFileInfo(p).fileName());
        a->setToolTip(p);
        connect(a, &QAction::triggered, this, [this, p] { openPath(p); });
    }
    menuRecent_->addSeparator();
    menuRecent_->addAction(QStringLiteral("Clear this list"), this, [this] {
        settings_.remove(QStringLiteral("recent"));
        rebuildRecent();
    });
}

void MainWindow::rememberRecent(const QString &path)
{
    QStringList recent = settings_.value(QStringLiteral("recent")).toStringList();
    recent.removeAll(path);
    recent.prepend(path);
    while (recent.size() > 8)
        recent.removeLast();
    settings_.setValue(QStringLiteral("recent"), recent);
    rebuildRecent();
}

void MainWindow::rebuildTrackMenus()
{
    menuAudio_->clear();
    menuSubs_->clear();
    audioIds_.clear();
    subIds_.clear();
    subExternal_.clear();
    auto *audioGroup = new QActionGroup(menuAudio_);
    auto *subGroup = new QActionGroup(menuSubs_);
    const auto addAudio = [&](const QString &text, int id, bool current, bool enabled) {
        QAction *a = menuAudio_->addAction(text);
        a->setCheckable(true);
        a->setChecked(current);
        a->setEnabled(enabled);
        audioGroup->addAction(a);
        audioIds_ << id;
        connect(a, &QAction::triggered, this, [this, id] {
            if (disc_->active())
                disc_->switchAudio(id);
            else
                player_->setAudioTrack(id);
            for (QAction *x : menuAudio_->actions())
                x->setChecked(x == sender());
        });
    };
    const auto addSub = [&](const QString &text, int id, bool external, bool current, bool enabled) {
        QAction *a = menuSubs_->addAction(text);
        a->setCheckable(true);
        a->setChecked(current);
        a->setEnabled(enabled);
        subGroup->addAction(a);
        subIds_ << id;
        subExternal_ << external;
        connect(a, &QAction::triggered, this, [this, id, external] {
            if (disc_->active())
                disc_->switchSubtitle(id);
            else
                player_->setSubtitleTrack(id, external);
        });
    };
    bool hasAudio = false, hasSubs = false;
    if (disc_->active()) {
        const auto audio = disc_->audioTracks();
        const auto subs = disc_->subtitleTracks();
        const bool audioOk = !disc_->locked(QStringLiteral("audio"));
        const bool subOk = !disc_->locked(QStringLiteral("subtitle"));
        for (const DiscTrack &t : audio) {
            QStringList detail;
            if (!t.purpose.isEmpty())
                detail << t.purpose;
            if (!t.codec.isEmpty())
                detail << t.codec.toUpper();
            if (t.channels > 0)
                detail << channelsText(t.channels);
            addAudio(trackLabel(t.language, detail.join(QStringLiteral(", "))), t.n, t.n == disc_->currentAudio(), audioOk);
        }
        addSub(QStringLiteral("Off"), -1, false, disc_->currentSubtitle() < 0, subOk);
        for (const DiscTrack &t : subs)
            addSub(trackLabel(t.language, t.purpose), t.n, t.external, t.n == disc_->currentSubtitle(), subOk);
        hasAudio = audio.size() > 1;
        hasSubs = !subs.isEmpty();
    } else if (player_->hasMedia()) {
        const auto audio = player_->audioTracks();
        const auto subs = player_->subtitleTracks();
        const int curA = player_->currentAudioTrack(), curS = player_->currentSubtitleTrack();
        const bool curExt = player_->currentSubtitleExternal();
        for (const TrackInfo &t : audio) {
            QStringList detail;
            if (!t.title.isEmpty())
                detail << t.title;
            if (!t.codec.isEmpty())
                detail << t.codec.toUpper();
            if (t.channels > 0)
                detail << channelsText(t.channels);
            addAudio(trackLabel(t.language, detail.join(QStringLiteral(", "))), t.index, t.index == curA, true);
        }
        addSub(QStringLiteral("Off"), -1, false, curS < 0, true);
        for (const TrackInfo &t : subs) {
            QStringList detail;
            if (!t.title.isEmpty())
                detail << t.title;
            if (t.forced)
                detail << QStringLiteral("forced");
            addSub(trackLabel(t.language, detail.join(QStringLiteral(", "))), t.index, t.external, t.index == curS && t.external == curExt, true);
        }
        hasAudio = audio.size() > 1;
        hasSubs = !subs.isEmpty();
    }
    if (menuAudio_->isEmpty())
        menuAudio_->addAction(QStringLiteral("No audio tracks"))->setEnabled(false);
    bar_->setHasTracks(hasAudio, hasSubs);
}

void MainWindow::rebuildDiscMenus()
{
    const bool on = disc_->active();
    menuDisc_->setEnabled(on);
    menuTitles_->clear();
    menuChapters_->clear();
    menuAngles_->clear();
    if (!on)
        return;
    for (const DiscTitle &t : disc_->titles()) {
        QString text = QStringLiteral("Title %1, %2").arg(t.n).arg(ControlBar::formatTime(t.duration));
        if (t.main)
            text += QStringLiteral(" (main feature)");
        QAction *a = menuTitles_->addAction(text);
        const int n = t.n;
        connect(a, &QAction::triggered, this, [this, n] { disc_->playTitle(n, 0); });
    }
    const auto chapters = disc_->chapters();
    menuChapters_->setEnabled(!chapters.isEmpty() && !disc_->locked(QStringLiteral("chapter")));
    for (int i = 0; i < chapters.size(); i++) {
        QAction *a = menuChapters_->addAction(QStringLiteral("Chapter %1, %2").arg(i + 1).arg(ControlBar::formatTime(chapters[i])));
        connect(a, &QAction::triggered, this, [this, i] { disc_->playChapter(i); });
    }
    const int angles = disc_->angles();
    menuAngles_->setEnabled(angles > 1 && !disc_->locked(QStringLiteral("angle")));
    auto *angleGroup = new QActionGroup(menuAngles_);
    for (int i = 1; i <= angles; i++) {
        QAction *a = menuAngles_->addAction(QStringLiteral("Angle %1").arg(i));
        a->setCheckable(true);
        a->setChecked(i == disc_->angle());
        angleGroup->addAction(a);
        connect(a, &QAction::triggered, this, [this, i] { disc_->switchAngle(i); });
    }
}

// ---------------------------------------------------------------------- opening

void MainWindow::openDialog()
{
    const QString p = QFileDialog::getOpenFileName(this, QStringLiteral("Open a file"), settings_.value(QStringLiteral("lastDir")).toString(),
        QStringLiteral("Media and disc images (*.mkv *.mp4 *.m4v *.mov *.avi *.webm *.ts *.m2ts *.mts *.mpg *.mpeg *.vob *.wmv *.flv *.ogv *.3gp *.mp3 *.m4a *.aac *.flac *.wav *.ogg *.oga *.opus *.wma *.ac3 *.dts *.iso *.img);;All files (*)"));
    if (!p.isEmpty())
        openPath(p);
}

void MainWindow::openFolderDialog()
{
    const QString p = QFileDialog::getExistingDirectory(this, QStringLiteral("Open a disc folder"), settings_.value(QStringLiteral("lastDir")).toString());
    if (!p.isEmpty())
        openPath(p);
}

void MainWindow::openPath(const QString &path)
{
    message_->hide();
    settings_.setValue(QStringLiteral("lastDir"), QFileInfo(path).absolutePath());
    const QString locale = QLocale::system().name().left(2);
    QJsonObject prefs{{"menuLanguage", locale},
                      {"audioLanguage", settings_.value(QStringLiteral("audioLanguage"), locale).toString()},
                      {"subtitleLanguage", settings_.value(QStringLiteral("subtitleLanguage")).toString()},
                      {"lowPower", QThread::idealThreadCount() < 4}};
    setStatus(QStringLiteral("Opening…"));
    if (disc_->active())
        disc_->close();
    player_->stop();
    view_->clearPicture();
    view_->setOverlays({});
    view_->setAspect({});
    engine_->call(QStringLiteral("open"), {{"path", QDir::toNativeSeparators(path)}, {"prefs", prefs}}, [this, path](const EngineReply &r) {
        if (!r.ok()) {
            if (r.code == QLatin1String("helper-lost") || !engine_->running()) {
                // Ordinary files don't need the helper.
                const QString suffix = QFileInfo(path).suffix().toLower();
                if (QFileInfo(path).isFile() && suffix != QLatin1String("iso") && suffix != QLatin1String("img")) {
                    isFile_ = true;
                    currentPath_ = path;
                    ended_ = false;
                    start_->hide();
                    MediaSpec spec;
                    spec.filePath = path;
                    player_->setPaused(false);
                    player_->open(spec);
                    return;
                }
            }
            setStatus({});
            showMessage(r.error.isEmpty() ? QStringLiteral("This couldn't be opened.") : r.error);
            return;
        }
        const QJsonObject o = r.result.toObject();
        const QString kind = o.value(QStringLiteral("kind")).toString();
        currentPath_ = path;
        ended_ = false;
        start_->hide();
        bar_->setHasMedia(true);
        rememberRecent(path);
        if (kind == QLatin1String("file")) {
            isFile_ = true;
            bar_->setDiscMode(false);
            bar_->setSeekable(true);
            bar_->setChapters({});
            setWindowTitle(QStringLiteral("%1 - WittyMediaPlayer").arg(QFileInfo(path).fileName()));
            MediaSpec spec;
            spec.filePath = path;
            player_->setPaused(false);
            player_->open(spec);
            setStatus(QStringLiteral("Playing %1").arg(QFileInfo(path).fileName()));
        } else {
            isFile_ = false;
            bar_->setDiscMode(true);
            disc_->start(o);
            setWindowTitle(QStringLiteral("%1 - WittyMediaPlayer").arg(disc_->label().isEmpty() ? QFileInfo(path).fileName() : disc_->label()));
            const QStringList notes = disc_->notes();
            if (!notes.isEmpty())
                setStatus(notes.first());
        }
        bar_->setPlaying(true);
        view_->setFocus();
    });
}

void MainWindow::stopPlayback(bool showStart)
{
    if (disc_->active())
        disc_->close();
    player_->stop();
    view_->clearPicture();
    view_->setOverlays({});
    audioCaption_->hide();
    currentPath_.clear();
    isFile_ = false;
    bar_->setHasMedia(false);
    bar_->setPlaying(false);
    bar_->setDiscMode(false);
    setWindowTitle(QStringLiteral("WittyMediaPlayer"));
    rebuildTrackMenus();
    rebuildDiscMenus();
    if (showStart) {
        if (fullscreen_)
            setFullscreen(false);
        start_->show();
        refreshDrives();
    }
    updateHitButtons();
}

void MainWindow::makeImage(const QString &source)
{
    QString from = source;
    if (from.isEmpty() && !currentPath_.isEmpty())
        from = currentPath_;
    auto *dlg = new ImageDialog(engine_, from, settings_.value(QStringLiteral("imageDir")).toString(), this);
    connect(dlg, &ImageDialog::folderChosen, this, [this](const QString &dir) { settings_.setValue(QStringLiteral("imageDir"), dir); });
    dlg->setAttribute(Qt::WA_DeleteOnClose);
    dlg->open();
}

// ---------------------------------------------------------------------- input

void MainWindow::togglePlay()
{
    if (!player_->hasMedia())
        return;
    if (disc_->active() && !disc_->togglePauseAllowed())
        return;
    if (ended_) {
        ended_ = false;
        player_->seek(0);
        player_->setPaused(false);
        bar_->setPlaying(true);
        return;
    }
    const bool nowPaused = !player_->paused();
    player_->setPaused(nowPaused);
    bar_->setPlaying(!nowPaused);
    setStatus(nowPaused ? QStringLiteral("Paused") : QStringLiteral("Playing"));
}

void MainWindow::setVolumePercent(int percent)
{
    volume_ = qBound(0, percent, 100);
    // Loudness is heard on a curve, so the slider's middle sounds like the middle.
    const double v = muted_ ? 0.0 : std::pow(volume_ / 100.0, 2.0);
    player_->setVolume(v);
    bar_->setVolume(volume_, muted_);
}

void MainWindow::cycleAudio()
{
    if (audioIds_.size() < 2)
        return;
    const int cur = disc_->active() ? disc_->currentAudio() : player_->currentAudioTrack();
    const int at = audioIds_.indexOf(cur);
    const int next = audioIds_[(at + 1) % audioIds_.size()];
    if (disc_->active())
        disc_->switchAudio(next);
    else
        player_->setAudioTrack(next);
    QTimer::singleShot(300, this, &MainWindow::rebuildTrackMenus);
}

void MainWindow::cycleSubtitle()
{
    if (subIds_.size() < 2)
        return;
    int at = 0;
    if (disc_->active()) {
        at = subIds_.indexOf(disc_->currentSubtitle());
    } else {
        const int cur = player_->currentSubtitleTrack();
        const bool ext = player_->currentSubtitleExternal();
        for (int i = 0; i < subIds_.size(); i++)
            if (subIds_[i] == cur && subExternal_[i] == ext)
                at = i;
    }
    const int k = (qMax(at, 0) + 1) % subIds_.size();
    if (disc_->active())
        disc_->switchSubtitle(subIds_[k]);
    else
        player_->setSubtitleTrack(subIds_[k], subExternal_[k]);
    QTimer::singleShot(300, this, &MainWindow::rebuildTrackMenus);
}

bool MainWindow::handleKey(int key, Qt::KeyboardModifiers)
{
    const bool discOn = disc_->active();
    const bool menuOn = discOn && disc_->inMenu();
    const bool has = player_->hasMedia();
    switch (key) {
    case Qt::Key_Up:
    case Qt::Key_Down:
    case Qt::Key_Left:
    case Qt::Key_Right: {
        if (menuOn) {
            disc_->move(key == Qt::Key_Up ? QStringLiteral("up") : key == Qt::Key_Down ? QStringLiteral("down") : key == Qt::Key_Left ? QStringLiteral("left") : QStringLiteral("right"));
            return true;
        }
        if (key == Qt::Key_Up)
            setVolumePercent(volume_ + 5);
        else if (key == Qt::Key_Down)
            setVolumePercent(volume_ - 5);
        else if (has) {
            const double d = key == Qt::Key_Right ? 10 : -10;
            if (discOn)
                disc_->seekBy(d);
            else
                player_->seek(player_->position() + d);
        }
        return true;
    }
    case Qt::Key_Return:
    case Qt::Key_Enter:
        if (menuOn) {
            disc_->pressSelected();
            return true;
        }
        return false;
    case Qt::Key_Space:
    case Qt::Key_K:
    case Qt::Key_MediaTogglePlayPause:
    case Qt::Key_MediaPlay:
    case Qt::Key_MediaPause:
        togglePlay();
        return true;
    case Qt::Key_MediaStop:
        stopPlayback();
        return true;
    case Qt::Key_F:
        setFullscreen(!fullscreen_);
        return true;
    case Qt::Key_Escape:
        if (discOn && disc_->popupOpen()) {
            disc_->back();
            return true;
        }
        if (message_->isVisible()) {
            message_->hide();
            return true;
        }
        if (fullscreen_) {
            setFullscreen(false);
            return true;
        }
        return false;
    case Qt::Key_M:
        if (discOn)
            disc_->togglePopup();
        else {
            muted_ = !muted_;
            setVolumePercent(volume_);
        }
        return true;
    case Qt::Key_T:
        if (discOn)
            disc_->titleMenu();
        return discOn;
    case Qt::Key_A:
        cycleAudio();
        return true;
    case Qt::Key_S:
        cycleSubtitle();
        return true;
    case Qt::Key_N:
    case Qt::Key_PageDown:
    case Qt::Key_MediaNext:
        if (discOn)
            disc_->nextChapter();
        return discOn;
    case Qt::Key_P:
    case Qt::Key_PageUp:
    case Qt::Key_MediaPrevious:
        if (discOn)
            disc_->prevChapter();
        return discOn;
    case Qt::Key_Backspace:
        if (discOn)
            disc_->back();
        return discOn;
    default:
        if (key >= Qt::Key_0 && key <= Qt::Key_9) {
            const int d = key - Qt::Key_0;
            if (discOn)
                disc_->digit(d);
            else if (has && player_->duration() > 0)
                player_->seek(player_->duration() * d / 10.0); // a tenth of the way per key
            return has;
        }
    }
    return false;
}

bool MainWindow::eventFilter(QObject *o, QEvent *e)
{
    if (e->type() == QEvent::KeyPress) {
        // Dialogs, menus and open pop-ups keep their own keys.
        if (QApplication::activePopupWidget() || QApplication::activeModalWidget())
            return QMainWindow::eventFilter(o, e);
        auto *k = static_cast<QKeyEvent *>(e);
        if (k->modifiers() & (Qt::ControlModifier | Qt::AltModifier | Qt::MetaModifier))
            return QMainWindow::eventFilter(o, e);
        QWidget *focus = QApplication::focusWidget();
        // The focused control keeps what it needs: arrows on the sliders, and Space
        // and Enter on a button.
        const bool arrow = k->key() == Qt::Key_Left || k->key() == Qt::Key_Right || k->key() == Qt::Key_Up || k->key() == Qt::Key_Down || k->key() == Qt::Key_Home || k->key() == Qt::Key_End;
        const bool activate = k->key() == Qt::Key_Space || k->key() == Qt::Key_Return || k->key() == Qt::Key_Enter;
        if (focus && (qobject_cast<QAbstractSlider *>(focus)) && arrow)
            return QMainWindow::eventFilter(o, e);
        if (focus && qobject_cast<QAbstractButton *>(focus) && activate && !hits_.contains(static_cast<QAbstractButton *>(focus)))
            return QMainWindow::eventFilter(o, e);
        if (focus && (focus == start_ || start_->isAncestorOf(focus)))
            return QMainWindow::eventFilter(o, e);
        if (focus && message_->isAncestorOf(focus) && k->key() != Qt::Key_Escape)
            return QMainWindow::eventFilter(o, e);
        if (handleKey(k->key(), k->modifiers()))
            return true;
    }
    if (e->type() == QEvent::MouseMove && fullscreen_)
        showControls();
    if (e->type() == QEvent::MouseButtonDblClick && o == view_) {
        setFullscreen(!fullscreen_);
        return true;
    }
    if (e->type() == QEvent::MouseButtonRelease && o == view_ && !disc_->inMenu() && player_->hasMedia()) {
        const auto *m = static_cast<QMouseEvent *>(e);
        if (m->button() == Qt::LeftButton)
            togglePlay();
    }
    return QMainWindow::eventFilter(o, e);
}

void MainWindow::mouseMoveEvent(QMouseEvent *e)
{
    if (fullscreen_)
        showControls();
    QMainWindow::mouseMoveEvent(e);
}

void MainWindow::showControls()
{
    if (!bar_->isVisible())
        bar_->show();
    unsetCursor();
    hideControls_.start();
}

void MainWindow::setFullscreen(bool on)
{
    fullscreen_ = on;
    bar_->setFullscreen(on);
    menuBar()->setVisible(!on);
    if (on) {
        showFullScreen();
        showControls();
    } else {
        showNormal();
        hideControls_.stop();
        bar_->show();
        unsetCursor();
    }
    layoutOverlays();
}

void MainWindow::dragEnterEvent(QDragEnterEvent *e)
{
    if (e->mimeData()->hasUrls())
        e->acceptProposedAction();
}

void MainWindow::dropEvent(QDropEvent *e)
{
    const auto urls = e->mimeData()->urls();
    if (urls.isEmpty())
        return;
    const QString p = urls.first().toLocalFile();
    if (!p.isEmpty()) {
        e->acceptProposedAction();
        activateWindow();
        openPath(p);
    }
}

bool MainWindow::nativeEvent(const QByteArray &type, void *message, qintptr *result)
{
#ifdef Q_OS_WIN
    if (type == "windows_generic_MSG") {
        const auto *msg = static_cast<MSG *>(message);
        // A disc went in or came out: never plays by itself, only updates the list.
        if (msg->message == WM_DEVICECHANGE && (msg->wParam == DBT_DEVICEARRIVAL || msg->wParam == DBT_DEVICEREMOVECOMPLETE)) {
            driveWatch_.singleShot(800, this, &MainWindow::refreshDrives);
        }
    }
#endif
    return QMainWindow::nativeEvent(type, message, result);
}

// ---------------------------------------------------------------------- layout

void MainWindow::resizeEvent(QResizeEvent *e)
{
    QMainWindow::resizeEvent(e);
    layoutOverlays();
}

void MainWindow::layoutOverlays()
{
    const QRect all = central_->rect();
    const int barH = bar_->sizeHint().height();
    if (fullscreen_) {
        view_->setGeometry(all);
    } else {
        view_->setGeometry(0, 0, all.width(), qMax(0, all.height() - barH));
    }
    bar_->setGeometry(0, all.height() - barH, all.width(), barH);
    bar_->raise();
    start_->setGeometry(0, 0, all.width(), qMax(0, all.height() - (fullscreen_ ? 0 : barH)));
    if (start_->isVisible())
        start_->raise();
    const int w = qMin(520, all.width() - 40);
    message_->setFixedWidth(w);
    message_->adjustSize();
    message_->move((all.width() - w) / 2, (view_->height() - message_->height()) / 2);
    if (message_->isVisible())
        message_->raise();
    audioCaption_->setGeometry(view_->rect());
    updateHitButtons();
}

void MainWindow::updateHitButtons()
{
    const auto buttons = disc_->inMenu() ? disc_->buttons() : QVector<DiscButton>{};
    while (hits_.size() > buttons.size())
        delete hits_.takeLast();
    while (hits_.size() < buttons.size()) {
        auto *h = new HitButton(view_);
        hits_ << h;
    }
    const QRectF pic = view_->pictureRect();
    const QSize ms = disc_->menuSize();
    const double sx = ms.width() > 0 ? pic.width() / ms.width() : 1;
    const double sy = ms.height() > 0 ? pic.height() / ms.height() : 1;
    for (int i = 0; i < buttons.size(); i++) {
        const DiscButton &b = buttons[i];
        auto *h = static_cast<HitButton *>(hits_[i]);
        h->disconnect(this);
        h->setGeometry(QRect(int(pic.x() + b.rect.x() * sx), int(pic.y() + b.rect.y() * sy), qMax(4, int(b.rect.width() * sx)), qMax(4, int(b.rect.height() * sy))));
        h->setAccessibleName(QStringLiteral("Menu option %1 of %2").arg(i + 1).arg(buttons.size()));
        h->selected = b.n == disc_->selected();
        h->drawOutline = !b.lit;
        const int n = b.n;
        connect(h, &QAbstractButton::clicked, this, [this, n] { disc_->press(n); });
        h->show();
        h->update();
        // Moving the mouse onto a button selects it.
        h->setProperty("discButton", n);
    }
}

int MainWindow::menuButtonCount() const
{
    return hits_.size();
}

void MainWindow::setStatus(const QString &text)
{
    status_ = text;
    announcer_->setText(text);
    QAccessibleEvent ev(announcer_, QAccessible::NameChanged);
    QAccessible::updateAccessibility(&ev);
}

QString MainWindow::shownMessage() const
{
    return message_->isVisible() ? message_->label->text() : QString();
}

void MainWindow::showMessage(const QString &text, bool stopsPlayback)
{
    if (stopsPlayback)
        stopPlayback();
    setStatus(text);
    message_->say(text);
    layoutOverlays();
}

void MainWindow::onTick()
{
    if (!player_->hasMedia())
        return;
    const double pos = player_->position();
    if (disc_->active()) {
        if (disc_->inTitle())
            bar_->setPosition(pos, disc_->titleDuration());
        else
            bar_->setPosition(pos, player_->duration());
    } else {
        bar_->setPosition(pos, player_->duration());
        updateFileSubtitles();
    }
    if (!isFile_ && !disc_->active())
        return;
    // Mouse over a disc's button selects it.
    if (disc_->inMenu()) {
        for (QAbstractButton *h : hits_) {
            if (h->underMouse()) {
                const int n = h->property("discButton").toInt();
                if (n != disc_->selected())
                    disc_->hover(n);
                break;
            }
        }
    }
}

void MainWindow::updateFileSubtitles()
{
    const auto cues = player_->activeCues();
    uint64_t sig = 14695981039346656037ULL;
    for (const SubtitleCue &c : cues)
        sig = (sig ^ c.id) * 1099511628211ULL;
    if (sig == subSig_)
        return;
    subSig_ = sig;
    QVector<Overlay> over;
    if (player_->isAudioOnly()) {
        const QImage art = player_->coverArt();
        if (!art.isNull())
            over.push_back({art.convertToFormat(QImage::Format_RGBA8888_Premultiplied), QRectF(0.25, 0.1, 0.5, 0.8), 1});
    }
    const FramePtr f = view_->currentFrame();
    const QSize frame = f ? QSize(f->width, f->height) : QSize(1280, 720);
    for (const SubtitleCue &c : cues) {
        if (!c.image.isNull())
            over.push_back({c.image, c.rect, (quint64(1) << 62) | c.id});
        else
            over.push_back(textOverlay(c.text, frame, c.id));
    }
    view_->setOverlays(over);
}

} // namespace wmp
