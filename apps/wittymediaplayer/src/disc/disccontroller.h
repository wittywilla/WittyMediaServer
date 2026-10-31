// Runs a disc the way the server's web player does: applies each presentation from
// the engine, holds stills, loops menus, keeps the disc's own timers, and sends the
// viewer's keys back along the disc's button links.
#pragma once

#include "../engine/engineclient.h"
#include "../media/player.h"
#include "../video/videoview.h"

#include <QHash>
#include <QJsonObject>
#include <QObject>
#include <QTimer>
#include <QVector>
#include <functional>
#include <map>

class QAudioSink;
class QBuffer;

namespace wmp {

struct DiscButton {
    int n = 0;
    QRect rect; // in menu pixels
    int up = 0, down = 0, left = 0, right = 0;
    bool autoAction = false;
    bool lit = false;
    int frames = 0;
    double fps = 0;
    int num = 0;
    int soundSelect = 0, soundPress = 0;
};

struct DiscTrack {
    int n = 0;
    QString language, codec, purpose;
    int channels = 0;
    int id = 0;
    bool external = false;
};

struct DiscTitle {
    int n = 0;
    double duration = 0;
    int chapters = 0;
    bool main = false;
};

class DiscController : public QObject {
    Q_OBJECT
public:
    DiscController(EngineClient *engine, Player *player, VideoView *view, QObject *parent = nullptr);

    // Takes the answer to an "open" call.
    void start(const QJsonObject &reply);
    void close();
    bool active() const { return active_; }

    QString kind() const { return info_.value(QStringLiteral("kind")).toString(); }
    QString label() const { return info_.value(QStringLiteral("label")).toString(); }
    QStringList notes() const;
    QVector<DiscTitle> titles() const { return titles_; }
    int mainTitle() const { return info_.value(QStringLiteral("mainTitle")).toInt(); }

    QString presentationKind() const { return kind_; }
    bool inTitle() const { return kind_ == QLatin1String("title"); }
    bool inMenu() const;
    bool hasPopup() const;
    bool popupOpen() const { return popupOpen_; }
    int ttn() const { return title_.value(QStringLiteral("ttn")).toInt(); }
    QVector<double> chapters() const { return chapters_; }
    int currentChapter(double pos) const;
    double titleDuration() const;
    QVector<DiscTrack> audioTracks() const { return audio_; }
    QVector<DiscTrack> subtitleTracks() const { return subs_; }
    int currentAudio() const { return title_.value(QStringLiteral("audioStream")).toInt(-1); }
    int currentSubtitle() const { return title_.value(QStringLiteral("subtitleStream")).toInt(-1); }
    int angle() const { return title_.value(QStringLiteral("angle")).toInt(1); }
    int angles() const { return title_.value(QStringLiteral("angles")).toInt(1); }
    QVector<DiscButton> buttons() const { return buttons_; }
    int selected() const { return selected_; }
    QSize menuSize() const { return menuSize_; }
    bool locked(const QString &what) const;
    void setObeyLocks(bool on) { obeyLocks_ = on; }
    bool obeyLocks() const { return obeyLocks_; }
    void setShowForced(bool on);
    bool showForced() const { return showForced_; }
    // A clip the viewer may skip: nothing to press, and it isn't a held choice.
    bool canSkip() const;

    // The viewer.
    void move(const QString &dir);
    void pressSelected();
    void press(int n);
    void hover(int n);
    void digit(int d);
    void rootMenu();
    void titleMenu();
    void resume();
    void back();
    void togglePopup();
    void skip();
    void nextChapter();
    void prevChapter();
    void seekTo(double sec);
    void seekBy(double delta);
    void playTitle(int ttn, int chapter = 0);
    void playChapter(int chapter);
    void switchAudio(int n);
    void switchSubtitle(int n);
    void switchAngle(int n);
    // Plays the disc's main feature, whatever the menus are doing.
    void playMain() { playTitle(mainTitle(), 0); }
    bool togglePauseAllowed() const { return !locked(QStringLiteral("pause")); }

signals:
    void changed();          // tracks, chapters, kind: rebuild what shows them
    void buttonsChanged();   // the menu's hit areas or selection
    void stopped(const QString &reason, bool isProtected);
    void message(const QString &text);
    void rememberAudio(const QString &language);
    void rememberSubtitle(const QString &language, bool off);

private:
    void apply(const QJsonObject &p);
    void act(const QJsonObject &action, bool quiet = false);
    void clipEnded();
    void onTick();
    void refreshOverlay();
    void refreshLayer();
    void loadArt();
    void fetchArt(int art, const QString &kind, int n, int frame);
    void updateSubtitleStream();
    void pollExternalSubs();
    void playSound(int id);
    double pos() const;
    QJsonObject withPosition(QJsonObject a) const;

    EngineClient *engine_;
    Player *player_;
    VideoView *view_;
    bool active_ = false;
    QJsonObject info_;
    QVector<DiscTitle> titles_;

    QString kind_;
    int serial_ = 0;
    QString token_;
    QJsonObject media_, menu_, title_;
    QVector<DiscButton> buttons_;
    QVector<DiscTrack> audio_, subs_;
    QVector<double> chapters_;
    QSize menuSize_;
    int selected_ = 0;
    int artKey_ = 0;
    bool showButtons_ = true;
    bool popupOpen_ = false;
    bool obeyLocks_ = true;
    bool showForced_ = true;
    bool forcedDone_ = false;
    bool endedFor_ = false;
    int hlFrame_ = 0;
    int effectIndex_ = -1;
    bool seekBusy_ = false;
    double seekQueued_ = -1;

    QTimer tick_, stillTimer_, navTimer_, popupTimer_, hlTimer_, effectTimer_;

    // Pictures from the engine, by art key.
    QHash<quint64, QImage> art_;
    QHash<int, QByteArray> sounds_;
    uint64_t subShown_ = 0;
    QVector<Overlay> lastSubs_;

    struct ExternalEvent {
        double start = 0, end = 0;
        QRectF rect;
        QString text, pic;
        bool forced = false;
        QImage image;
    };
    QVector<ExternalEvent> external_;
    int externalWindow_ = -1;
    int externalStream_ = -1;
    bool externalBusy_ = false;
    uint64_t overlaySignature_ = 0;
};

} // namespace wmp
