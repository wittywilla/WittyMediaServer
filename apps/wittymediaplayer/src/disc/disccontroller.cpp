#include "disccontroller.h"

#include "../ui/subtitlepaint.h"

#include <QAudioFormat>
#include <QAudioSink>
#include <QBuffer>
#include <QJsonArray>
#include <QMediaDevices>
#include <cmath>

namespace wmp {

namespace {

quint64 artId(int art, int kind, int n, int frame)
{
    return (quint64(uint32_t(art)) << 32) | (quint64(uint32_t(kind)) << 28) | (quint64(uint32_t(n & 0xfffff)) << 8) | quint64(uint32_t(frame & 0xff));
}

constexpr int kOverlay = 1, kHighlight = 2, kEffect = 3;
constexpr double kExternalWindow = 120.0;

} // namespace

DiscController::DiscController(EngineClient *engine, Player *player, VideoView *view, QObject *parent)
    : QObject(parent), engine_(engine), player_(player), view_(view)
{
    tick_.setInterval(100);
    connect(&tick_, &QTimer::timeout, this, &DiscController::onTick);
    stillTimer_.setSingleShot(true);
    navTimer_.setSingleShot(true);
    popupTimer_.setSingleShot(true);
    effectTimer_.setSingleShot(true);
    connect(&stillTimer_, &QTimer::timeout, this, [this] { act({{"type", "ended"}}); });
    connect(&navTimer_, &QTimer::timeout, this, [this] { act({{"type", "tick"}}); });
    connect(&popupTimer_, &QTimer::timeout, this, [this] {
        popupOpen_ = false;
        refreshLayer();
    });
    connect(&hlTimer_, &QTimer::timeout, this, [this] {
        for (const DiscButton &b : buttons_)
            if (b.n == selected_ && b.frames > 1) {
                hlFrame_ = (hlFrame_ + 1) % b.frames;
                fetchArt(artKey_, QStringLiteral("highlight"), b.n, hlFrame_);
                refreshOverlay();
            }
    });
    connect(&effectTimer_, &QTimer::timeout, this, [this] {
        const QJsonArray fx = menu_.value(QStringLiteral("inEffect")).toArray();
        effectIndex_++;
        if (effectIndex_ >= fx.size()) {
            effectIndex_ = -1;
        } else {
            fetchArt(artKey_, QStringLiteral("effect"), effectIndex_, 0);
            effectTimer_.start(qMax(16, int(fx[effectIndex_].toDouble() * 1000)));
        }
        refreshOverlay();
    });
    connect(player_, &Player::ended, this, &DiscController::clipEnded);
}

void DiscController::start(const QJsonObject &reply)
{
    active_ = true;
    info_ = reply.value(QStringLiteral("info")).toObject();
    titles_.clear();
    for (const auto &v : info_.value(QStringLiteral("titles")).toArray()) {
        const QJsonObject o = v.toObject();
        titles_.push_back({o.value("n").toInt(), o.value("durationSec").toDouble(), o.value("chapters").toInt(), o.value("main").toBool()});
    }
    serial_ = 0;
    token_.clear();
    art_.clear();
    sounds_.clear();
    tick_.start();
    apply(reply.value(QStringLiteral("presentation")).toObject());
}

void DiscController::close()
{
    active_ = false;
    tick_.stop();
    stillTimer_.stop();
    navTimer_.stop();
    popupTimer_.stop();
    hlTimer_.stop();
    effectTimer_.stop();
    kind_.clear();
    buttons_.clear();
    audio_.clear();
    subs_.clear();
    chapters_.clear();
    title_ = menu_ = media_ = {};
    art_.clear();
    external_.clear();
    view_->setOverlays({});
    view_->setAspect({});
    // One that has gone has nothing to close, and starting it again only to say so holds up the window.
    if (engine_->running())
        engine_->call(QStringLiteral("close"), {});
    emit buttonsChanged();
    emit changed();
}

QStringList DiscController::notes() const
{
    QStringList out;
    for (const auto &v : info_.value(QStringLiteral("notes")).toArray())
        out << v.toString();
    return out;
}

double DiscController::pos() const
{
    return player_->position();
}

QJsonObject DiscController::withPosition(QJsonObject a) const
{
    a.insert(QStringLiteral("positionSec"), inTitle() ? pos() : 0.0);
    return a;
}

void DiscController::act(const QJsonObject &action, bool quiet)
{
    if (!active_)
        return;
    const int serial = serial_;
    engine_->call(QStringLiteral("action"), action, [this, quiet, serial](const EngineReply &r) {
        if (!active_)
            return;
        if (!r.ok()) {
            emit stopped(r.error, false);
            return;
        }
        const QJsonObject p = r.result.toObject();
        // A quiet highlight move only matters if the disc moved on meanwhile.
        if (quiet && p.value(QStringLiteral("serial")).toInt() == serial)
            return;
        apply(p);
    });
}

void DiscController::apply(const QJsonObject &p)
{
    const QString newKind = p.value(QStringLiteral("kind")).toString();
    const int serial = p.value(QStringLiteral("serial")).toInt();
    const QJsonObject media = p.value(QStringLiteral("media")).toObject();
    const QString token = media.value(QStringLiteral("token")).toString();

    if (p.contains(QStringLiteral("pause")))
        player_->setPaused(p.value(QStringLiteral("pause")).toBool());

    const auto takeMenu = [&] {
        menu_ = p.value(QStringLiteral("menu")).toObject();
        buttons_.clear();
        for (const auto &v : menu_.value(QStringLiteral("buttons")).toArray()) {
            const QJsonObject b = v.toObject();
            DiscButton d;
            d.n = b.value("n").toInt();
            d.rect = QRect(QPoint(b.value("x1").toInt(), b.value("y1").toInt()), QPoint(b.value("x2").toInt(), b.value("y2").toInt()));
            d.up = b.value("up").toInt();
            d.down = b.value("down").toInt();
            d.left = b.value("left").toInt();
            d.right = b.value("right").toInt();
            d.autoAction = b.value("auto").toBool();
            d.lit = b.value("lit").toBool();
            d.frames = b.value("frames").toInt();
            d.fps = b.value("fps").toDouble();
            d.num = b.value("num").toInt();
            d.soundSelect = b.value("soundSelect").toInt();
            d.soundPress = b.value("soundPress").toInt();
            buttons_.push_back(d);
        }
        menuSize_ = QSize(menu_.value("width").toInt(), menu_.value("height").toInt());
        selected_ = menu_.value("selected").toInt();
        artKey_ = menu_.value("art").toInt();
    };

    // The same screen and stream: only the highlight (or a message) changed.
    if (serial == serial_ && token == token_ && newKind == kind_) {
        const bool closed = p.value(QStringLiteral("menu")).toObject().value(QStringLiteral("closed")).toBool();
        takeMenu();
        title_ = p.value(QStringLiteral("title")).toObject();
        if (closed)
            popupOpen_ = false;
        updateSubtitleStream();
        loadArt();
        refreshLayer();
        emit changed();
        return;
    }

    const bool sameStream = !token.isEmpty() && token == token_;
    const bool wasTitle = inTitle();
    popupOpen_ = false;
    stillTimer_.stop();
    navTimer_.stop();
    popupTimer_.stop();
    effectTimer_.stop();
    effectIndex_ = -1;
    forcedDone_ = false;
    endedFor_ = false;
    hlFrame_ = 0;
    serial_ = serial;
    token_ = token;
    kind_ = newKind;
    media_ = media;
    title_ = p.value(QStringLiteral("title")).toObject();
    takeMenu();

    audio_.clear();
    subs_.clear();
    chapters_.clear();
    for (const auto &v : title_.value(QStringLiteral("audio")).toArray()) {
        const QJsonObject o = v.toObject();
        audio_.push_back({o.value("n").toInt(), o.value("language").toString(), o.value("codec").toString(), o.value("purpose").toString(), o.value("channels").toInt(), o.value("id").toInt(), false});
    }
    for (const auto &v : title_.value(QStringLiteral("subtitles")).toArray()) {
        const QJsonObject o = v.toObject();
        subs_.push_back({o.value("n").toInt(), o.value("language").toString(), o.value("codec").toString(), o.value("purpose").toString(), 0, o.value("id").toInt(), o.value("external").toBool()});
    }
    for (const auto &v : title_.value(QStringLiteral("chapters")).toArray())
        chapters_.push_back(v.toDouble());

    if (newKind == QLatin1String("stop")) {
        player_->stop();
        view_->clearPicture();
        view_->setOverlays({});
        hlTimer_.stop();
        emit buttonsChanged();
        emit changed();
        emit stopped(p.value(QStringLiteral("reason")).toString(), p.value(QStringLiteral("protected")).toBool());
        return;
    }

    const QString reason = p.value(QStringLiteral("reason")).toString();
    if (!reason.isEmpty() && reason.contains(QLatin1String("Java")))
        emit message(reason);

    const double fromButtons = menu_.value(QStringLiteral("buttonsFromSec")).toDouble();
    showButtons_ = !(fromButtons > 0.3);
    view_->setAspect(inTitle() ? title_.value(QStringLiteral("aspect")).toString() : menu_.value(QStringLiteral("aspect")).toString());

    if (sameStream && wasTitle && inTitle()) {
        // The same stream from another place (a pop-up menu chose a chapter): seek.
        const double start = title_.value(QStringLiteral("startSec")).toDouble();
        if (std::abs(pos() - start) > 1.5)
            seekTo(start);
    } else if (media.value(QStringLiteral("type")).toString() == QLatin1String("clip")) {
        MediaSpec spec = MediaSpec::fromJson(media);
        for (const auto &v : title_.value(QStringLiteral("palette")).toArray())
            spec.subPalette << v.toString();
        player_->open(spec);
        player_->setPaused(false);
    }
    externalWindow_ = -1;
    externalStream_ = -1;
    external_.clear();
    updateSubtitleStream();

    const double timer = p.value(QStringLiteral("timerSec")).toDouble();
    if (timer > 0)
        navTimer_.start(int(timer * 1000) + 150);

    // Button animation and the page's opening transition.
    hlTimer_.stop();
    loadArt();
    const QJsonArray fx = menu_.value(QStringLiteral("inEffect")).toArray();
    if (!fx.isEmpty() && (!inTitle() || popupOpen_)) {
        effectIndex_ = 0;
        fetchArt(artKey_, QStringLiteral("effect"), 0, 0);
        effectTimer_.start(qMax(16, int(fx[0].toDouble() * 1000)));
    }
    refreshLayer();
    emit changed();
}

void DiscController::loadArt()
{
    if (menu_.isEmpty())
        return;
    if (menu_.value(QStringLiteral("hasOverlay")).toBool())
        fetchArt(artKey_, QStringLiteral("overlay"), 0, 0);
    if (menu_.value(QStringLiteral("hasHighlights")).toBool())
        for (const DiscButton &b : buttons_)
            if (b.lit)
                fetchArt(artKey_, QStringLiteral("highlight"), b.n, 0);
}

void DiscController::fetchArt(int art, const QString &kind, int n, int frame)
{
    const int k = kind == QLatin1String("overlay") ? kOverlay : kind == QLatin1String("highlight") ? kHighlight : kEffect;
    const quint64 id = artId(art, k, n, frame);
    if (art_.contains(id))
        return;
    art_.insert(id, QImage()); // being fetched
    // Old menus' pictures go once this menu has its own.
    if (art_.size() > 512)
        art_.clear();
    engine_->call(QStringLiteral("art"), {{"art", art}, {"kind", kind}, {"n", n}, {"frame", frame}}, [this, id](const EngineReply &r) {
        if (!active_ || !r.ok())
            return;
        QImage img = QImage::fromData(r.blob, "PNG");
        if (!img.isNull()) {
            art_.insert(id, img.convertToFormat(QImage::Format_RGBA8888_Premultiplied));
            refreshOverlay();
        }
    });
}

bool DiscController::inMenu() const
{
    if (buttons_.isEmpty())
        return false;
    return (kind_ == QLatin1String("menu") && showButtons_) || (hasPopup() && popupOpen_);
}

bool DiscController::hasPopup() const
{
    return inTitle() && menu_.value(QStringLiteral("popup")).toBool() && !buttons_.isEmpty();
}

bool DiscController::canSkip() const
{
    if (kind_ != QLatin1String("menu"))
        return false;
    return buttons_.isEmpty() || (!media_.value(QStringLiteral("loop")).toBool() && media_.value(QStringLiteral("stillSec")).toDouble() != 0);
}

bool DiscController::locked(const QString &what) const
{
    if (!obeyLocks_)
        return false;
    for (const auto &v : title_.value(QStringLiteral("locks")).toArray())
        if (v.toString() == what)
            return true;
    return false;
}

double DiscController::titleDuration() const
{
    return title_.value(QStringLiteral("durationSec")).toDouble();
}

int DiscController::currentChapter(double p) const
{
    int cur = 0;
    for (int i = 0; i < chapters_.size(); i++)
        if (chapters_[i] <= p + 1)
            cur = i;
    return cur;
}

void DiscController::clipEnded()
{
    if (!active_ || endedFor_)
        return;
    if (media_.value(QStringLiteral("loop")).toBool()) {
        player_->restart();
        return;
    }
    const double after = media_.value(QStringLiteral("stillSec")).toDouble();
    if (after < 0)
        return; // hold the last picture until the viewer acts
    endedFor_ = true;
    stillTimer_.start(int(after * 1000));
}

void DiscController::onTick()
{
    if (!active_)
        return;
    const double t = pos();
    if (kind_ == QLatin1String("menu") && !menu_.isEmpty()) {
        // The disc's buttons work from buttonsFromSec until buttonsUntilSec; a
        // looping clip brings them back when it starts over.
        const double from = menu_.value(QStringLiteral("buttonsFromSec")).toDouble();
        const double until = menu_.value(QStringLiteral("buttonsUntilSec")).toDouble();
        const bool on = (from <= 0 || t >= from) && (until <= 0 || t < until);
        if (on != showButtons_ && (on || until > 0)) {
            showButtons_ = on;
            refreshLayer();
        }
    }
    if (!menu_.isEmpty() && !forcedDone_) {
        const int forced = menu_.value(QStringLiteral("forcedAction")).toInt();
        const double at = menu_.value(QStringLiteral("actionAtSec")).toDouble();
        if (forced > 0 && at > 0 && t >= at) {
            forcedDone_ = true;
            press(forced);
        }
    }
    if (inTitle()) {
        pollExternalSubs();
        refreshOverlay();
    }
}

void DiscController::refreshLayer()
{
    // Animate the selected button's highlight.
    hlTimer_.stop();
    if (inMenu()) {
        for (const DiscButton &b : buttons_)
            if (b.n == selected_ && b.frames > 1 && b.fps > 0)
                hlTimer_.start(int(1000.0 / qMax(1.0, b.fps)));
    }
    if (hasPopup() && popupOpen_) {
        const double secs = menu_.value(QStringLiteral("userTimeoutSec")).toDouble();
        if (secs > 0)
            popupTimer_.start(int(secs * 1000));
    }
    refreshOverlay();
    emit buttonsChanged();
}

void DiscController::refreshOverlay()
{
    QVector<Overlay> out;
    const QRectF whole(0, 0, 1, 1);
    const bool menuOn = !menu_.isEmpty() && (kind_ == QLatin1String("menu") || (hasPopup() && popupOpen_));
    if (menuOn) {
        if (effectIndex_ >= 0) {
            const quint64 id = artId(artKey_, kEffect, effectIndex_, 0);
            const QImage img = art_.value(id);
            if (!img.isNull())
                out.push_back({img, whole, id});
        } else if (menu_.value(QStringLiteral("hasOverlay")).toBool()) {
            const quint64 id = artId(artKey_, kOverlay, 0, 0);
            const QImage img = art_.value(id);
            if (!img.isNull())
                out.push_back({img, whole, id});
        }
        if (inMenu()) {
            for (const DiscButton &b : buttons_) {
                if (b.n != selected_ || !b.lit)
                    continue;
                const int frame = b.frames > 1 ? hlFrame_ : 0;
                quint64 id = artId(artKey_, kHighlight, b.n, frame);
                QImage img = art_.value(id);
                if (img.isNull()) { // not here yet: the first frame will do
                    id = artId(artKey_, kHighlight, b.n, 0);
                    img = art_.value(id);
                }
                if (!img.isNull())
                    out.push_back({img, whole, id});
            }
        }
    }
    if (inTitle()) {
        const QSize frame(title_.value(QStringLiteral("frameW")).toInt(), title_.value(QStringLiteral("frameH")).toInt());
        const auto cues = player_->activeCues();
        for (const SubtitleCue &c : cues) {
            if (!c.image.isNull())
                out.push_back({c.image, c.rect, (quint64(1) << 62) | c.id});
            else
                out.push_back(textOverlay(c.text, frame, c.id));
        }
        const double t = pos();
        for (int i = 0; i < external_.size(); i++) {
            const ExternalEvent &e = external_[i];
            if (e.start > t || e.end <= t)
                continue;
            if (currentSubtitle() < 0 && !(e.forced && showForced_))
                continue;
            if (!e.image.isNull())
                out.push_back({e.image, e.rect, (quint64(3) << 61) | quint64(uint32_t(i))});
            else if (!e.text.isEmpty())
                out.push_back(textOverlay(e.text, frame, (quint64(1) << 60) | quint64(uint32_t(i))));
        }
    }
    // Rebuild only when what shows has changed.
    uint64_t sig = 1469598103934665603ULL;
    for (const Overlay &o : out)
        sig = (sig ^ o.key) * 1099511628211ULL;
    if (sig == overlaySignature_)
        return;
    overlaySignature_ = sig;
    view_->setOverlays(out);
}

void DiscController::updateSubtitleStream()
{
    if (!inTitle()) {
        player_->setDiscSubtitle(0, false);
        return;
    }
    const int n = currentSubtitle();
    const int forced = title_.value(QStringLiteral("forcedStream")).toInt(-1);
    const int want = n >= 0 ? n : (showForced_ ? forced : -1);
    int id = 0;
    for (const DiscTrack &t : subs_)
        if (t.n == want)
            id = t.external ? 0 : t.id;
    player_->setDiscSubtitle(id, n < 0);
}

void DiscController::pollExternalSubs()
{
    const int n = currentSubtitle();
    const int forced = title_.value(QStringLiteral("forcedStream")).toInt(-1);
    const int want = n >= 0 ? n : (showForced_ ? forced : -1);
    bool ext = false;
    for (const DiscTrack &t : subs_)
        if (t.n == want && t.external)
            ext = true;
    if (!ext) {
        if (!external_.isEmpty()) {
            external_.clear();
            externalWindow_ = externalStream_ = -1;
        }
        return;
    }
    const int window = int(pos() / kExternalWindow);
    if (externalBusy_ || (window == externalWindow_ && want == externalStream_))
        return;
    externalBusy_ = true;
    externalWindow_ = window;
    externalStream_ = want;
    const QSize frame(title_.value(QStringLiteral("frameW")).toInt(), title_.value(QStringLiteral("frameH")).toInt());
    engine_->call(QStringLiteral("subs"), {{"ttn", ttn()}, {"stream", want}, {"from", window * kExternalWindow}, {"to", (window + 1) * kExternalWindow}}, [this, frame](const EngineReply &r) {
        externalBusy_ = false;
        if (!active_ || !r.ok())
            return;
        external_.clear();
        const QJsonArray events = r.result.toArray();
        for (const auto &v : events) {
            const QJsonObject o = v.toObject();
            ExternalEvent e;
            e.start = o.value("start").toDouble();
            e.end = o.value("end").toDouble();
            e.forced = o.value("forced").toBool();
            e.text = o.value("text").toString();
            e.pic = o.value("pic").toString();
            if (frame.width() > 0 && frame.height() > 0)
                e.rect = QRectF(double(o.value("x").toInt()) / frame.width(), double(o.value("y").toInt()) / frame.height(), double(o.value("w").toInt()) / frame.width(), double(o.value("h").toInt()) / frame.height());
            external_.push_back(e);
            if (!e.pic.isEmpty()) {
                const int idx = external_.size() - 1;
                const QString pic = e.pic;
                engine_->call(QStringLiteral("subpic"), {{"key", pic}}, [this, idx, pic](const EngineReply &rp) {
                    if (active_ && rp.ok() && idx < external_.size() && external_[idx].pic == pic)
                        external_[idx].image = QImage::fromData(rp.blob, "PNG").convertToFormat(QImage::Format_RGBA8888_Premultiplied);
                });
            }
        }
    });
}

// ---------------------------------------------------------------------- viewer

void DiscController::playSound(int id)
{
    if (id <= 0 || !menu_.value(QStringLiteral("hasSounds")).toBool())
        return;
    const auto play = [](const QByteArray &wav) {
        // A WAV file: find its format and its samples.
        if (wav.size() < 44 || !wav.startsWith("RIFF"))
            return;
        qsizetype at = 12;
        QAudioFormat f;
        QByteArray data;
        while (at + 8 <= wav.size()) {
            const QByteArray tag = wav.mid(at, 4);
            const quint32 len = quint32(uchar(wav[at + 4])) | quint32(uchar(wav[at + 5])) << 8 | quint32(uchar(wav[at + 6])) << 16 | quint32(uchar(wav[at + 7])) << 24;
            if (tag == "fmt " && len >= 16 && at + 8 + 16 <= wav.size()) {
                const auto *p = reinterpret_cast<const uchar *>(wav.constData() + at + 8);
                f.setChannelCount(p[2] | p[3] << 8);
                f.setSampleRate(int(p[4] | p[5] << 8 | p[6] << 16 | quint32(p[7]) << 24));
                f.setSampleFormat((p[14] | p[15] << 8) == 8 ? QAudioFormat::UInt8 : QAudioFormat::Int16);
            } else if (tag == "data") {
                data = wav.mid(at + 8, len);
            }
            at += 8 + qsizetype(len) + (len & 1);
        }
        const QAudioDevice dev = QMediaDevices::defaultAudioOutput();
        if (data.isEmpty() || f.sampleRate() <= 0 || dev.isNull() || !dev.isFormatSupported(f))
            return;
        auto *sink = new QAudioSink(dev, f);
        auto *buf = new QBuffer(sink);
        buf->setData(data);
        buf->open(QIODevice::ReadOnly);
        QObject::connect(sink, &QAudioSink::stateChanged, sink, [sink](QAudio::State s) {
            if (s == QAudio::IdleState || s == QAudio::StoppedState)
                sink->deleteLater();
        });
        sink->start(buf);
    };
    auto it = sounds_.constFind(id);
    if (it != sounds_.constEnd()) {
        play(it.value());
        return;
    }
    engine_->call(QStringLiteral("sound"), {{"id", id - 1}}, [this, id, play](const EngineReply &r) {
        if (active_ && r.ok()) {
            sounds_.insert(id, r.blob);
            play(r.blob);
        }
    });
}

void DiscController::press(int n)
{
    for (const DiscButton &b : buttons_)
        if (b.n == n)
            playSound(b.soundPress);
    act({{"type", "press"}, {"button", n}, {"positionSec", inTitle() ? pos() : 0.0}});
}

void DiscController::pressSelected()
{
    if (inMenu() && selected_)
        press(selected_);
}

void DiscController::hover(int n)
{
    if (n == selected_ || !inMenu())
        return;
    for (const DiscButton &b : buttons_) {
        if (b.n == n) {
            playSound(b.soundSelect);
            selected_ = n;
            hlFrame_ = 0;
            refreshLayer();
            act({{"type", "select"}, {"button", n}}, true);
            return;
        }
    }
}

void DiscController::move(const QString &dir)
{
    if (!inMenu() || buttons_.isEmpty())
        return;
    const DiscButton *cur = &buttons_[0];
    for (const DiscButton &b : buttons_)
        if (b.n == selected_)
            cur = &b;
    const int next = dir == QLatin1String("up") ? cur->up : dir == QLatin1String("down") ? cur->down : dir == QLatin1String("left") ? cur->left : cur->right;
    for (const DiscButton &b : buttons_) {
        if (b.n == next && b.n != cur->n) {
            if (b.autoAction) {
                press(b.n);
                return;
            }
            // At once, here; the disc hears about it behind the scenes.
            playSound(b.soundSelect);
            selected_ = b.n;
            hlFrame_ = 0;
            refreshLayer();
            act({{"type", "select"}, {"button", b.n}}, true);
            return;
        }
    }
}

void DiscController::digit(int d)
{
    if (inMenu()) {
        for (const DiscButton &b : buttons_)
            if ((b.num ? b.num : b.n) == d) {
                press(b.n);
                return;
            }
    } else if (d >= 1 && d <= chapters_.size()) {
        seekTo(chapters_[d - 1]);
    }
}

void DiscController::rootMenu()
{
    act(withPosition({{"type", "menu"}, {"menu", "root"}}));
}

void DiscController::titleMenu()
{
    act(withPosition({{"type", "menu"}, {"menu", "title"}}));
}

void DiscController::resume()
{
    act({{"type", "resume"}});
}

void DiscController::back()
{
    if (popupOpen_) {
        popupOpen_ = false;
        refreshLayer();
    } else {
        act(withPosition({{"type", "key"}, {"key", "return"}}));
    }
}

void DiscController::togglePopup()
{
    if (!hasPopup()) {
        rootMenu();
        return;
    }
    popupOpen_ = !popupOpen_;
    refreshLayer();
}

void DiscController::skip()
{
    act({{"type", "ended"}});
}

void DiscController::seekTo(double sec)
{
    if (!inTitle() || locked(QStringLiteral("seek")))
        return;
    sec = std::clamp(sec, 0.0, qMax(0.0, titleDuration() - 0.5));
    // Only the newest place matters: a drag along the bar mustn't queue up seeks,
    // least of all on a real drive.
    if (seekBusy_) {
        seekQueued_ = sec;
        return;
    }
    seekBusy_ = true;
    engine_->call(QStringLiteral("locate"), {{"sec", sec}}, [this, sec](const EngineReply &r) {
        seekBusy_ = false;
        if (active_ && r.ok()) {
            const QJsonObject o = r.result.toObject();
            player_->seekDisc(qint64(o.value("offset").toDouble()), o.value("sec").toDouble(), qint64(o.value("pts").toDouble(-1)), sec);
        }
        if (seekQueued_ >= 0) {
            const double again = seekQueued_;
            seekQueued_ = -1;
            seekTo(again);
        }
    });
}

void DiscController::seekBy(double delta)
{
    seekTo(pos() + delta);
}

void DiscController::nextChapter()
{
    if (!inTitle()) {
        act({{"type", "chapter"}, {"chapter", 1}});
        return;
    }
    if (locked(QStringLiteral("skip")))
        return;
    const double p = pos();
    const int cur = currentChapter(p);
    // On the last scene, or a one-scene clip such as a warning or trailer, Next
    // moves on to what the disc plays next, like a disc player.
    if (cur >= chapters_.size() - 1) {
        skip();
        return;
    }
    seekTo(chapters_[cur + 1]);
}

void DiscController::prevChapter()
{
    if (!inTitle()) {
        act({{"type", "chapter"}, {"chapter", -1}});
        return;
    }
    if (locked(QStringLiteral("skip")))
        return;
    const double p = pos();
    // A slideshow's first picture and earlier: the disc's previous item.
    if (chapters_.size() <= 1 && p < 3) {
        act({{"type", "key"}, {"key", "prev"}});
        return;
    }
    if (chapters_.isEmpty())
        return;
    const int cur = currentChapter(p);
    if (p - chapters_[cur] > 3)
        seekTo(chapters_[cur]);
    else
        seekTo(chapters_[qMax(0, cur - 1)]);
}

void DiscController::playTitle(int ttn, int chapter)
{
    if (!active_)
        return;
    engine_->call(QStringLiteral("title"), {{"ttn", ttn}, {"chapter", chapter}}, [this](const EngineReply &r) {
        if (active_ && r.ok())
            apply(r.result.toObject());
    });
}

void DiscController::playChapter(int chapter)
{
    if (chapter >= 0 && chapter < chapters_.size())
        seekTo(chapters_[chapter]);
}

void DiscController::switchAudio(int n)
{
    if (locked(QStringLiteral("audio")))
        return;
    for (const DiscTrack &t : audio_)
        if (t.n == n)
            emit rememberAudio(t.language);
    // A new stream at the same moment.
    act({{"type", "audio"}, {"stream", n}, {"positionSec", pos()}});
}

void DiscController::switchSubtitle(int n)
{
    if (locked(QStringLiteral("subtitle")))
        return;
    QString lang;
    for (const DiscTrack &t : subs_)
        if (t.n == n)
            lang = t.language;
    emit rememberSubtitle(lang, n < 0);
    act({{"type", "subtitle"}, {"stream", qMax(0, n)}, {"on", n >= 0}});
}

void DiscController::switchAngle(int n)
{
    if (locked(QStringLiteral("angle")))
        return;
    act({{"type", "angle"}, {"stream", n}, {"positionSec", pos()}});
}

void DiscController::setShowForced(bool on)
{
    showForced_ = on;
    updateSubtitleStream();
}

} // namespace wmp
