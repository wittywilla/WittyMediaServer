#include "vlcplayback.h"

#include "extentreader.h"
#include "ffmpeg.h"

#include <QDir>
#include <QFile>
#include <QFileInfo>
#include <QSettings>
#include <QThread>
#include <cstring>

namespace wmp {

// The parts of libVLC 3 used here, declared by hand so that nothing is needed at build
// time: without a VLC on the computer this code simply stays unused.
extern "C" {
typedef int (*vlc_open_cb)(void *, void **, uint64_t *);
typedef ptrdiff_t (*vlc_read_cb)(void *, unsigned char *, size_t);
typedef int (*vlc_seek_cb)(void *, uint64_t);
typedef void (*vlc_close_cb)(void *);
typedef void *(*vlc_lock_cb)(void *, void **);
typedef void (*vlc_unlock_cb)(void *, void *, void *const *);
typedef void (*vlc_display_cb)(void *, void *);
typedef unsigned (*vlc_format_cb)(void **, char *, unsigned *, unsigned *, unsigned *, unsigned *);
typedef void (*vlc_cleanup_cb)(void *);
struct vlc_track_desc {
    int id;
    char *name;
    vlc_track_desc *next;
};
}

struct VlcApi {
    void *(*new_)(int, const char *const *) = nullptr;
    void (*release)(void *) = nullptr;
    const char *(*get_version)() = nullptr;
    void *(*media_new_callbacks)(void *, vlc_open_cb, vlc_read_cb, vlc_seek_cb, vlc_close_cb, void *) = nullptr;
    void (*media_add_option)(void *, const char *) = nullptr;
    void (*media_release)(void *) = nullptr;
    void *(*player_new_from_media)(void *) = nullptr;
    void (*player_release)(void *) = nullptr;
    int (*player_play)(void *) = nullptr;
    void (*player_stop)(void *) = nullptr;
    void (*player_set_pause)(void *, int) = nullptr;
    int64_t (*player_get_time)(void *) = nullptr;
    void (*player_set_time)(void *, int64_t) = nullptr;
    int64_t (*player_get_length)(void *) = nullptr;
    float (*player_get_position)(void *) = nullptr;
    void (*player_set_position)(void *, float) = nullptr;
    int (*player_get_state)(void *) = nullptr;
    unsigned (*player_has_vout)(void *) = nullptr;
    int (*video_get_size)(void *, unsigned, unsigned *, unsigned *) = nullptr;
    void (*video_set_callbacks)(void *, vlc_lock_cb, vlc_unlock_cb, vlc_display_cb, void *) = nullptr;
    void (*video_set_format_callbacks)(void *, vlc_format_cb, vlc_cleanup_cb) = nullptr;
    int (*audio_set_volume)(void *, int) = nullptr;
    vlc_track_desc *(*audio_get_track_description)(void *) = nullptr;
    int (*audio_get_track)(void *) = nullptr;
    int (*audio_set_track)(void *, int) = nullptr;
    vlc_track_desc *(*video_get_spu_description)(void *) = nullptr;
    int (*video_get_spu)(void *) = nullptr;
    int (*video_set_spu)(void *, int) = nullptr;
    void (*track_description_list_release)(vlc_track_desc *) = nullptr;
};

namespace {

enum VlcState { StateNothing, StateOpening, StateBuffering, StatePlaying, StatePaused, StateStopped, StateEnded, StateError };

// What VLC may read a file as: never its disc modules, which can use libdvdcss. A file
// that's really a disc image with the wrong name just won't open.
const char kFileDemuxers[] = ":demux=mkv,mp4,avi,ps,ts,ogg,flac,wav,asf,mpgv,mpga,es,aiff,caf,avformat,none";

const char *const kArgs[] = {
    "--intf=dummy",
    "--no-video-title-show",
    "--no-stats",
    "--no-lua",
    "--no-metadata-network-access",
    "--no-sub-autodetect-file",
    "--no-playlist-autostart",
    "--no-one-instance",
    "--no-snapshot-preview",
    "--avcodec-hw=any",
    "--deinterlace=-1",
    "--deinterlace-mode=yadif",
    "--quiet",
};

int align32(int v)
{
    return (v + 31) & ~31;
}

} // namespace

struct VlcSession {
    VlcApi *api = nullptr;
    void *media = nullptr;
    void *player = nullptr;
    MediaSpec spec;
    std::unique_ptr<ExtentReader> reader;
    VideoSink *sink = nullptr;
    Player *owner = nullptr;
    std::atomic<bool> stopping{false};
    std::atomic<int64_t> framesShown{0};
    std::atomic<int> width{0}, height{0};
    // VLC's buffers can be taller than the picture in them.
    std::atomic<int> visibleW{0}, visibleH{0};
    std::atomic<bool> gotFrame{false};
    std::atomic<bool> video{false};
    int pitchY = 0, pitchC = 0, linesY = 0, linesC = 0;
    AVRational sar{0, 1};
    bool started = false;
    bool sentOpened = false;
    bool sentEnded = false;
    bool startApplied = false;
    int wantSpu = -2;
    int playingTicks = 0;
    int lastTrackCount = -1;
    double pendingTarget = -1;

    // Read by VLC's threads.
    static int onOpen(void *opaque, void **data, uint64_t *size)
    {
        auto *s = static_cast<VlcSession *>(opaque);
        *data = s;
        *size = uint64_t(s->reader->size());
        s->reader->seek(0);
        return 0;
    }
    static ptrdiff_t onRead(void *data, unsigned char *buf, size_t len)
    {
        auto *s = static_cast<VlcSession *>(data);
        return s->reader->read(buf, int(qMin<size_t>(len, 1 << 24)));
    }
    static int onSeek(void *data, uint64_t off)
    {
        return static_cast<VlcSession *>(data)->reader->seek(int64_t(off)) >= 0 ? 0 : -1;
    }
    static void onClose(void *) {}

    // Pictures as ordinary frames, so menus and subtitles draw over them as usual.
    static unsigned onFormat(void **opaque, char *chroma, unsigned *w, unsigned *h, unsigned *pitches, unsigned *lines)
    {
        auto *s = static_cast<VlcSession *>(*opaque);
        std::memcpy(chroma, "I420", 4);
        s->width.store(int(*w));
        s->height.store(int(*h));
        s->pitchY = align32(int(*w));
        s->pitchC = align32(int(*w + 1) / 2);
        s->linesY = align32(int(*h));
        s->linesC = align32(int(*h + 1) / 2);
        pitches[0] = unsigned(s->pitchY);
        pitches[1] = pitches[2] = unsigned(s->pitchC);
        lines[0] = unsigned(s->linesY);
        lines[1] = lines[2] = unsigned(s->linesC);
        s->video.store(true);
        return 1;
    }
    static void onCleanup(void *) {}
    static void *onLock(void *opaque, void **planes)
    {
        auto *s = static_cast<VlcSession *>(opaque);
        AVFrame *f = av_frame_alloc();
        const int size = s->pitchY * s->linesY + 2 * s->pitchC * s->linesC;
        f->buf[0] = av_buffer_alloc(size + 64);
        f->format = AV_PIX_FMT_YUV420P;
        const int vw = s->visibleW.load(), vh = s->visibleH.load();
        f->width = vw > 0 && vw <= s->width.load() ? vw : s->width.load();
        f->height = vh > 0 && vh <= s->height.load() ? vh : s->height.load();
        f->sample_aspect_ratio = s->sar;
        f->data[0] = f->buf[0]->data;
        f->data[1] = f->data[0] + s->pitchY * s->linesY;
        f->data[2] = f->data[1] + s->pitchC * s->linesC;
        f->linesize[0] = s->pitchY;
        f->linesize[1] = f->linesize[2] = s->pitchC;
        for (int i = 0; i < 3; i++)
            planes[i] = f->data[i];
        return f;
    }
    static void onUnlock(void *, void *, void *const *) {}
    static void onDisplay(void *opaque, void *picture)
    {
        auto *s = static_cast<VlcSession *>(opaque);
        auto *f = static_cast<AVFrame *>(picture);
        if (s->stopping.load() || !s->sink) {
            av_frame_free(&f);
            return;
        }
        s->sink->presentFrame(shareFrame(f));
        s->framesShown++;
    }

    ~VlcSession()
    {
        if (player)
            api->player_release(player);
        if (media)
            api->media_release(media);
    }
};

VlcPlayback::VlcPlayback(Player *owner) : QObject(owner), owner_(owner)
{
    poll_.setInterval(100);
    connect(&poll_, &QTimer::timeout, this, &VlcPlayback::poll);
}

VlcPlayback::~VlcPlayback()
{
    stop();
    if (instance_)
        api_->release(instance_);
}

QString VlcPlayback::detectFolder()
{
#ifdef Q_OS_WIN
    const QString reg = QSettings(QStringLiteral("HKEY_LOCAL_MACHINE\\SOFTWARE\\VideoLAN\\VLC"), QSettings::NativeFormat).value(QStringLiteral("InstallDir")).toString();
    QStringList places;
    if (!reg.isEmpty())
        places << reg;
    places << qEnvironmentVariable("ProgramFiles") + QStringLiteral("\\VideoLAN\\VLC");
    for (const QString &p : places)
        if (QFile::exists(QDir(p).filePath(QStringLiteral("libvlc.dll"))))
            return QDir::toNativeSeparators(p);
#endif
    return {};
}

bool VlcPlayback::loaded() const
{
    return instance_ != nullptr;
}

QString VlcPlayback::version() const
{
    return version_;
}

bool VlcPlayback::load(const QString &folder, QString *why)
{
    if (instance_)
        return true;
    const auto fail = [&](const QString &m) {
        if (why)
            *why = m;
        return false;
    };
    QDir dir(folder);
#ifdef Q_OS_WIN
    const QString libName = QStringLiteral("libvlc.dll"), coreName = QStringLiteral("libvlccore.dll");
#else
    const QString libName = QStringLiteral("libvlc.so"), coreName = QStringLiteral("libvlccore.so");
#endif
    if (folder.isEmpty() || !QFile::exists(dir.filePath(libName)))
        return fail(QStringLiteral("VLC's libraries aren't in that folder."));
    core_.setFileName(dir.filePath(coreName));
    lib_.setFileName(dir.filePath(libName));
    if (!core_.load() || !lib_.load())
        return fail(QStringLiteral("VLC's libraries couldn't be loaded. A 64-bit VLC 3 is needed."));
    auto api = std::make_unique<VlcApi>();
    bool ok = true;
    const auto get = [&](auto &slot, const char *name) {
        slot = reinterpret_cast<std::decay_t<decltype(slot)>>(lib_.resolve(name));
        ok = ok && slot;
    };
    get(api->new_, "libvlc_new");
    get(api->release, "libvlc_release");
    get(api->get_version, "libvlc_get_version");
    get(api->media_new_callbacks, "libvlc_media_new_callbacks");
    get(api->media_add_option, "libvlc_media_add_option");
    get(api->media_release, "libvlc_media_release");
    get(api->player_new_from_media, "libvlc_media_player_new_from_media");
    get(api->player_release, "libvlc_media_player_release");
    get(api->player_play, "libvlc_media_player_play");
    get(api->player_stop, "libvlc_media_player_stop");
    get(api->player_set_pause, "libvlc_media_player_set_pause");
    get(api->player_get_time, "libvlc_media_player_get_time");
    get(api->player_set_time, "libvlc_media_player_set_time");
    get(api->player_get_length, "libvlc_media_player_get_length");
    get(api->player_get_position, "libvlc_media_player_get_position");
    get(api->player_set_position, "libvlc_media_player_set_position");
    get(api->player_get_state, "libvlc_media_player_get_state");
    get(api->player_has_vout, "libvlc_media_player_has_vout");
    get(api->video_get_size, "libvlc_video_get_size");
    get(api->video_set_callbacks, "libvlc_video_set_callbacks");
    get(api->video_set_format_callbacks, "libvlc_video_set_format_callbacks");
    get(api->audio_set_volume, "libvlc_audio_set_volume");
    get(api->audio_get_track_description, "libvlc_audio_get_track_description");
    get(api->audio_get_track, "libvlc_audio_get_track");
    get(api->audio_set_track, "libvlc_audio_set_track");
    get(api->video_get_spu_description, "libvlc_video_get_spu_description");
    get(api->video_get_spu, "libvlc_video_get_spu");
    get(api->video_set_spu, "libvlc_video_set_spu");
    get(api->track_description_list_release, "libvlc_track_description_list_release");
    if (!ok)
        return fail(QStringLiteral("This version of VLC isn't one WittyMediaPlayer knows how to use. VLC 3 is needed."));
    const QString v = QString::fromLatin1(api->get_version());
    if (!v.startsWith(QLatin1String("3.")))
        return fail(QStringLiteral("This is VLC %1. WittyMediaPlayer works with VLC 3.").arg(v.section(QLatin1Char(' '), 0, 0)));
    qputenv("VLC_PLUGIN_PATH", QDir::toNativeSeparators(dir.filePath(QStringLiteral("plugins"))).toLocal8Bit());
    instance_ = api->new_(int(sizeof(kArgs) / sizeof(kArgs[0])), kArgs);
    if (!instance_)
        return fail(QStringLiteral("VLC's libraries wouldn't start."));
    version_ = v.section(QLatin1Char(' '), 0, 0);
    api_ = std::move(api);
    return true;
}

int VlcPlayback::discStreamId(const QString &container, int id)
{
    if (container == QLatin1String("mpegts") || id == 0)
        return id; // a PID
    if (id >= 0x1c0)
        return id & 0xff; // 0x1e0 video, 0x1c0 MPEG audio
    return 0xbd00 | id;   // AC-3, DTS, LPCM and sub-pictures are in private stream 1
}

double VlcPlayback::timeAt(const QVector<Segment> &segments, double fraction)
{
    int64_t total = 0;
    for (const Segment &g : segments)
        total += g.bytes;
    if (total <= 0)
        return 0;
    int64_t at = int64_t(qBound(0.0, fraction, 1.0) * double(total));
    for (const Segment &g : segments) {
        if (at < g.bytes || &g == &segments.last())
            return g.startSec + (g.bytes > 0 ? double(at) / double(g.bytes) : 0) * g.durationSec;
        at -= g.bytes;
    }
    return 0;
}

void VlcPlayback::open(const MediaSpec &spec, VideoSink *sink)
{
    stop();
    auto s = std::make_shared<VlcSession>();
    s->api = api_.get();
    s->spec = spec;
    s->sink = sink;
    s->owner = owner_;
    // Everything goes through this player's reader: a file is one long range of itself.
    if (spec.isDisc()) {
        s->reader = std::make_unique<ExtentReader>(spec.files, spec.extents);
    } else {
        if (!QFile::exists(spec.filePath)) {
            emit owner_->failed(QStringLiteral("This file can't be found."));
            return;
        }
        s->reader = std::make_unique<ExtentReader>(QStringList{spec.filePath}, QVector<Extent>{{0, 0, QFileInfo(spec.filePath).size()}});
    }
    s->media = api_->media_new_callbacks(instance_, &VlcSession::onOpen, &VlcSession::onRead, &VlcSession::onSeek, &VlcSession::onClose, s.get());
    if (!s->media) {
        emit owner_->failed(QStringLiteral("VLC's libraries couldn't open this."));
        return;
    }
    if (spec.isDisc()) {
        api_->media_add_option(s->media, spec.container == QLatin1String("mpegts") ? ":demux=ts" : ":demux=ps");
        if (spec.audioId != 0)
            api_->media_add_option(s->media, QStringLiteral(":audio-track-id=%1").arg(discStreamId(spec.container, spec.audioId)).toLatin1().constData());
        else if (!spec.audioFirst)
            api_->media_add_option(s->media, ":no-audio");
        if (spec.videoId != 0)
            api_->media_add_option(s->media, QStringLiteral(":video-track-id=%1").arg(discStreamId(spec.container, spec.videoId)).toLatin1().constData());
        // The disc says its picture's shape, and this player scales to it.
        if (spec.aspect == QLatin1String("4:3") || spec.aspect == QLatin1String("16:9"))
            s->sar = AVRational{0, 1};
    } else {
        api_->media_add_option(s->media, kFileDemuxers);
    }
    s->player = api_->player_new_from_media(s->media);
    api_->video_set_callbacks(s->player, &VlcSession::onLock, &VlcSession::onUnlock, &VlcSession::onDisplay, s.get());
    api_->video_set_format_callbacks(s->player, &VlcSession::onFormat, &VlcSession::onCleanup);
    api_->audio_set_volume(s->player, int(volume_ * 100));
    api_->player_play(s->player);
    session_ = s;
    poll_.start();
}

void VlcPlayback::stop()
{
    poll_.stop();
    auto s = session_;
    session_.reset();
    if (!s)
        return;
    s->stopping.store(true);
    if (s->reader)
        s->reader->stop();
    // A drive stuck on a bad sector mustn't hold up the window.
    QThread *t = QThread::create([s] { s->api->player_stop(s->player); });
    connect(t, &QThread::finished, t, &QObject::deleteLater);
    t->start();
}

bool VlcPlayback::hasMedia() const
{
    return session_ != nullptr;
}

void VlcPlayback::setPaused(bool paused)
{
    paused_ = paused;
    if (session_)
        api_->player_set_pause(session_->player, paused ? 1 : 0);
}

void VlcPlayback::seek(double sec)
{
    if (!session_)
        return;
    session_->sentEnded = false;
    api_->player_set_time(session_->player, int64_t(qMax(0.0, sec) * 1000));
}

void VlcPlayback::seekDisc(int64_t offset, double target)
{
    auto s = session_;
    if (!s || s->reader->size() <= 0)
        return;
    s->sentEnded = false;
    s->pendingTarget = target;
    api_->player_set_position(s->player, float(double(offset) / double(s->reader->size())));
}

void VlcPlayback::restart()
{
    auto s = session_;
    if (!s)
        return;
    MediaSpec spec = s->spec;
    spec.startSec = 0;
    spec.startOffset = 0;
    open(spec, s->sink);
}

void VlcPlayback::setVolume(double v)
{
    volume_ = v;
    if (session_)
        api_->audio_set_volume(session_->player, int(v * 100));
}

void VlcPlayback::setAudioTrack(int id)
{
    if (session_)
        api_->audio_set_track(session_->player, id);
}

void VlcPlayback::setSubtitleTrack(int id)
{
    if (session_)
        api_->video_set_spu(session_->player, id);
}

void VlcPlayback::setDiscSubtitle(int streamId, bool forcedOnly)
{
    auto s = session_;
    if (!s)
        return;
    // VLC has no setting for only a disc's forced captions, so they're off with the rest.
    s->wantSpu = (streamId == 0 || forcedOnly) ? -1 : discStreamId(s->spec.container, streamId);
    applyPending();
}

void VlcPlayback::applyPending()
{
    auto s = session_;
    if (!s || s->wantSpu == -2 || api_->player_get_state(s->player) != StatePlaying)
        return;
    api_->video_set_spu(s->player, s->wantSpu);
    s->wantSpu = -2;
}

void VlcPlayback::poll()
{
    auto s = session_;
    if (!s)
        return;
    const int state = api_->player_get_state(s->player);
    if (state == StatePlaying || state == StatePaused) {
        if (!s->sentOpened) {
            s->sentOpened = true;
            emit owner_->opened();
            emit owner_->tracksChanged();
        }
        s->playingTicks++;
        unsigned vw = 0, vh = 0;
        if (s->video.load() && api_->video_get_size(s->player, 0, &vw, &vh) == 0) {
            s->visibleW.store(int(vw));
            s->visibleH.store(int(vh));
        }
        // A new stream starting part way through.
        if (!s->startApplied) {
            s->startApplied = true;
            if (s->spec.isDisc() && s->spec.startOffset > 0 && s->reader->size() > 0)
                api_->player_set_position(s->player, float(double(s->spec.startOffset) / double(s->reader->size())));
            if (paused_)
                api_->player_set_pause(s->player, 1);
        }
        applyPending();
        // Tracks appear as VLC finds them.
        if (!s->spec.isDisc()) {
            const int count = api_->player_has_vout(s->player) + audioTracks().size() + subtitleTracks().size();
            if (count != s->lastTrackCount) {
                s->lastTrackCount = count;
                emit owner_->tracksChanged();
            }
        }
    }
    if (s->framesShown.load() > 0 && !s->gotFrame.exchange(true))
        emit owner_->firstFrame();
    if (state == StateEnded && !s->sentEnded) {
        s->sentEnded = true;
        emit owner_->ended();
    } else if (state == StateError && !s->sentEnded) {
        s->sentEnded = true;
        emit owner_->failed(QStringLiteral("VLC's libraries couldn't play this."));
    }
}

double VlcPlayback::position() const
{
    auto s = session_;
    if (!s)
        return 0;
    if (s->spec.isDisc())
        return timeAt(s->spec.segments, double(api_->player_get_position(s->player)));
    return qMax<double>(0, double(api_->player_get_time(s->player)) / 1000.0);
}

double VlcPlayback::duration() const
{
    auto s = session_;
    if (!s)
        return 0;
    if (s->spec.isDisc())
        return s->spec.durationSec;
    return qMax<double>(0, double(api_->player_get_length(s->player)) / 1000.0);
}

bool VlcPlayback::isAudioOnly() const
{
    auto s = session_;
    return s && s->playingTicks > 15 && !s->video.load();
}

namespace {
QVector<TrackInfo> describeTracks(VlcApi *api, vlc_track_desc *list)
{
    QVector<TrackInfo> out;
    for (vlc_track_desc *d = list; d; d = d->next) {
        if (d->id < 0)
            continue; // VLC's own "disable" entry: the window has an Off of its own
        TrackInfo t;
        t.index = d->id;
        t.title = d->name ? QString::fromUtf8(d->name) : QString();
        out << t;
    }
    if (list)
        api->track_description_list_release(list);
    return out;
}
} // namespace

QVector<TrackInfo> VlcPlayback::audioTracks() const
{
    return session_ ? describeTracks(api_.get(), api_->audio_get_track_description(session_->player)) : QVector<TrackInfo>{};
}

QVector<TrackInfo> VlcPlayback::subtitleTracks() const
{
    return session_ ? describeTracks(api_.get(), api_->video_get_spu_description(session_->player)) : QVector<TrackInfo>{};
}

int VlcPlayback::currentAudioTrack() const
{
    return session_ ? api_->audio_get_track(session_->player) : -1;
}

int VlcPlayback::currentSubtitleTrack() const
{
    return session_ ? api_->video_get_spu(session_->player) : -1;
}

PlayerStats VlcPlayback::stats() const
{
    PlayerStats out;
    auto s = session_;
    if (!s)
        return out;
    out.framesShown = s->framesShown.load();
    out.framesDecoded = out.framesShown;
    out.width = s->visibleW.load() > 0 ? s->visibleW.load() : s->width.load();
    out.height = s->visibleH.load() > 0 ? s->visibleH.load() : s->height.load();
    out.badBlocks = s->reader ? s->reader->badBlocks() : 0;
    out.videoCodec = QStringLiteral("VLC %1").arg(version_);
    return out;
}

} // namespace wmp
