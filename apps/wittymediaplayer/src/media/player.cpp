#include "player.h"

#include "audiooutput.h"
#include "cdstream.h"
#include "extentreader.h"
#include "vlcplayback.h"

#include <QDir>
#include <QFile>
#include <QFileInfo>
#include <QJsonArray>
#include <QLocale>
#include <QRegularExpression>
#include <QThread>
#include <algorithm>
#include <chrono>
#include <cmath>
#include <condition_variable>
#include <cstring>
#include <deque>
#include <mutex>
#include <optional>
#include <thread>

namespace wmp {

MediaSpec MediaSpec::fromJson(const QJsonObject &m)
{
    MediaSpec s;
    s.token = m.value("token").toString();
    s.container = m.value("container").toString();
    for (const auto &f : m.value("files").toArray())
        s.files << f.toString();
    for (const auto &e : m.value("extents").toArray()) {
        const QJsonArray a = e.toArray();
        if (a.size() == 3)
            s.extents.push_back({a[0].toInt(), qint64(a[1].toDouble()), qint64(a[2].toDouble())});
    }
    for (const auto &g : m.value("segments").toArray()) {
        const QJsonObject o = g.toObject();
        s.segments.push_back({qint64(o.value("bytes").toDouble()), o.value("startSec").toDouble(), o.value("durationSec").toDouble(), qint64(o.value("basePts").toDouble(-1))});
    }
    s.videoId = m.value("video").toInt();
    s.audioId = m.value("audio").toInt();
    s.audioFirst = m.value("audioFirst").toBool();
    s.frontOnly = m.value("frontOnly").toBool();
    s.loop = m.value("loop").toBool();
    s.durationSec = m.value("durationSec").toDouble();
    s.stillSec = m.value("stillSec").toDouble();
    s.deinterlace = m.value("deinterlace").toBool();
    s.aspect = m.value("aspect").toString();
    s.startSec = m.value("startSec").toDouble();
    s.startOffset = qint64(m.value("startOffset").toDouble());
    s.startAt = m.value("startAt").toDouble();
    s.startPts = qint64(m.value("startPts").toDouble(-1));
    return s;
}

QString languageName(const QString &code)
{
    if (code.isEmpty() || code == QLatin1String("und"))
        return QStringLiteral("Unknown language");
    QLocale l(code);
    if (l.language() != QLocale::C)
        return QLocale::languageToString(l.language());
    // Three-letter codes QLocale doesn't take directly.
    const QLocale::Language lang = QLocale::codeToLanguage(code, QLocale::AnyLanguageCode);
    if (lang != QLocale::AnyLanguage && lang != QLocale::C)
        return QLocale::languageToString(lang);
    return code;
}

namespace {

using Clock = std::chrono::steady_clock;

// ---------------------------------------------------------------------- limits

// Only what a media file is. Playlist and reference formats (which can point
// anywhere) aren't built into this FFmpeg at all, and this list is a second check.
const char kFormats[] = "matroska,webm,mov,mp4,m4a,3gp,3g2,mj2,avi,mpegts,mpegtsraw,mpeg,mpegvideo,asf,flv,ogg,wav,flac,mp3,aac,ac3,eac3,dts,dtshd,truehd,mlp,ape,wv,aiff,tta,caf,nut,rm,vc1,h264,hevc,m4v";
const char kSubFormats[] = "srt,ass,webvtt";

constexpr int kMaxVideoQueue = 32 * 1024 * 1024;
constexpr int kMaxAudioQueue = 6 * 1024 * 1024;
constexpr int64_t kMaxSidecarBytes = 16 * 1024 * 1024;
constexpr size_t kMaxCues = 4000;

// ---------------------------------------------------------------------- queue

enum class ItemKind { Packet, Flush, Eof, Swap };

struct Item {
    ItemKind kind = ItemKind::Packet;
    AVPacket *pkt = nullptr;
    AVCodecContext *newCtx = nullptr; // Swap
};

class PacketQueue {
public:
    void push(Item it)
    {
        std::lock_guard<std::mutex> l(mu_);
        if (it.pkt)
            bytes_ += it.pkt->size;
        items_.push_back(it);
        cv_.notify_all();
    }
    // Waits for an item, or returns false once stopped.
    bool pop(Item *out, const std::atomic<bool> &stop)
    {
        std::unique_lock<std::mutex> l(mu_);
        cv_.wait(l, [&] { return stop.load() || !items_.empty(); });
        if (stop.load())
            return false;
        *out = items_.front();
        items_.pop_front();
        if (out->pkt)
            bytes_ -= out->pkt->size;
        cv_.notify_all();
        return true;
    }
    // Empties the queue, but keeps a decoder on its way to the thread that needs it.
    void clear()
    {
        std::lock_guard<std::mutex> l(mu_);
        std::deque<Item> keep;
        for (Item &i : items_) {
            if (i.kind == ItemKind::Swap)
                keep.push_back(i);
            else
                av_packet_free(&i.pkt);
        }
        items_.swap(keep);
        bytes_ = 0;
        cv_.notify_all();
    }
    void freeAll()
    {
        std::lock_guard<std::mutex> l(mu_);
        for (Item &i : items_) {
            av_packet_free(&i.pkt);
            avcodec_free_context(&i.newCtx);
        }
        items_.clear();
        bytes_ = 0;
    }
    int bytes() const
    {
        std::lock_guard<std::mutex> l(mu_);
        return bytes_;
    }
    bool empty() const
    {
        std::lock_guard<std::mutex> l(mu_);
        return items_.empty();
    }
    void wake()
    {
        std::lock_guard<std::mutex> l(mu_);
        cv_.notify_all();
    }

private:
    mutable std::mutex mu_;
    std::condition_variable cv_;
    std::deque<Item> items_;
    int bytes_ = 0;
};

enum AVPixelFormat pickHw(AVCodecContext *ctx, const enum AVPixelFormat *fmts);

AVHWDeviceType preferredHwType()
{
#ifdef _WIN32
    return AV_HWDEVICE_TYPE_D3D11VA;
#else
    return AV_HWDEVICE_TYPE_NONE; // VA-API and VideoToolbox come with those platforms' releases
#endif
}

bool hwWorthIt(AVCodecID id)
{
    switch (id) {
    case AV_CODEC_ID_H264:
    case AV_CODEC_ID_HEVC:
    case AV_CODEC_ID_VC1:
    case AV_CODEC_ID_WMV3:
    case AV_CODEC_ID_VP9:
    case AV_CODEC_ID_AV1:
        return true;
    default:
        return false; // MPEG-2 and older are cheap in software on anything
    }
}

} // namespace

// ---------------------------------------------------------------------- session

struct Session {
    Player *owner = nullptr;
    VideoSink *sink = nullptr;
    AudioOutput *audio = nullptr;
    MediaSpec spec;

    std::atomic<bool> stop{false};
    // Qt's own threads: these post to the window, and Qt only cleans up properly after
    // threads it knows about.
    QThread *demuxThread = nullptr, *videoThread = nullptr, *audioThread = nullptr;

    std::unique_ptr<ByteSource> reader;
    AVFormatContext *fmt = nullptr;
    std::atomic<int> vIndex{-1}, aIndex{-1}, sIndex{-1};
    // Streams stay put when the demuxer finds a new one; its list of them may not.
    std::atomic<AVStream *> vStream{nullptr}, aStream{nullptr};
    AVCodecContext *vctx = nullptr, *actx = nullptr, *sctx = nullptr;
    AVBufferRef *hwDevice = nullptr;
    PacketQueue vq, aq;

    // Requests, read by the demux thread.
    std::mutex reqMu;
    std::condition_variable reqCv;
    bool seekWanted = false;
    double seekSec = 0;
    bool seekByOffset = false;
    int64_t seekOffset = 0;
    double seekAt = 0;
    int64_t seekPts = -1;
    bool restartWanted = false;
    int wantAudio = -2, wantSub = -2; // files: -2 = no change
    bool wantSubExternal = false;
    std::atomic<int> discSubId{0};
    std::atomic<bool> forcedOnly{false};
    std::atomic<bool> discSubChanged{false};
    uint64_t seekGen = 1;
    std::atomic<uint64_t> audioGen{1};

    // Playing state.
    std::atomic<bool> paused{false};
    std::mutex pauseMu;
    std::condition_variable pauseCv;
    std::atomic<bool> eof{false};
    std::atomic<bool> videoDone{false}, audioDone{false};
    std::atomic<bool> endedSent{false};
    std::atomic<bool> gotFrame{false};
    std::atomic<double> dropVideoUntil{-1}, dropAudioUntil{-1};
    std::atomic<double> position{0};
    std::atomic<double> durationSec{0};
    std::atomic<bool> audioStarted{false};

    // The wall clock, when there is no sound to follow.
    mutable std::mutex clockMu;
    double wallBase = -1;
    Clock::time_point wallAt{};
    bool wallPaused = false;

    // Disc timeline.
    std::vector<int64_t> segStart;
    std::vector<double> segOffset;
    std::vector<char> segKnown;
    int curSeg = -1;
    double lastOffset = 0;
    bool haveLastOffset = false;
    double lastDemuxSec = -1;

    // Info for the window.
    mutable std::mutex infoMu;
    QVector<TrackInfo> audioTracks, subTracks;
    int currentAudio = -1, currentSub = -1;
    bool currentSubExternal = false;
    QString title;
    QImage cover;
    bool audioOnly = false;
    PlayerStats stats;

    // Subtitles.
    mutable std::mutex cueMu;
    std::deque<SubtitleCue> cues;
    QVector<QVector<SubtitleCue>> sidecars;
    uint64_t nextCue = 1;
    std::atomic<bool> subDecoderGeneric{false};

    // Stats.
    std::atomic<int64_t> framesDecoded{0}, framesShown{0}, framesDropped{0};

    ~Session() { release(); }

    bool audioOnlyNow() const { return vIndex.load() < 0; }

    void release()
    {
        vq.freeAll();
        aq.freeAll();
        avcodec_free_context(&vctx);
        avcodec_free_context(&actx);
        avcodec_free_context(&sctx);
        av_buffer_unref(&hwDevice);
        if (fmt) {
            if (reader)
                fmt->pb = nullptr; // the reader owns its context
            avformat_close_input(&fmt);
        }
    }

    void fail(const QString &message)
    {
        if (!stop.load())
            QMetaObject::invokeMethod(owner, [o = owner, message] { emit o->failed(message); }, Qt::QueuedConnection);
    }

    // ------------------------------------------------------------------ clock

    void setWall(double t)
    {
        std::lock_guard<std::mutex> l(clockMu);
        wallBase = t;
        wallAt = Clock::now();
    }
    void pauseWall(bool on)
    {
        std::lock_guard<std::mutex> l(clockMu);
        if (on && !wallPaused && wallBase >= 0) {
            wallBase += std::chrono::duration<double>(Clock::now() - wallAt).count();
        }
        wallPaused = on;
        wallAt = Clock::now();
    }
    double wall() const
    {
        std::lock_guard<std::mutex> l(clockMu);
        if (wallBase < 0)
            return -1;
        if (wallPaused)
            return wallBase;
        return wallBase + std::chrono::duration<double>(Clock::now() - wallAt).count();
    }
    double clock() const
    {
        if (aIndex >= 0 && !audioDone.load() && audioStarted.load()) {
            double a = audio->positionSec();
            if (a >= 0)
                return a;
        }
        return wall();
    }

    void waitWhilePaused()
    {
        if (!paused.load())
            return;
        std::unique_lock<std::mutex> l(pauseMu);
        pauseCv.wait(l, [&] { return stop.load() || !paused.load(); });
    }

    // ------------------------------------------------------------------ opening

    bool openDecoder(AVStream *st, AVCodecContext **out, bool wantHw)
    {
        const AVCodec *codec = avcodec_find_decoder(st->codecpar->codec_id);
        if (!codec)
            return false;
        AVCodecContext *c = avcodec_alloc_context3(codec);
        if (!c || avcodec_parameters_to_context(c, st->codecpar) < 0) {
            avcodec_free_context(&c);
            return false;
        }
        c->pkt_timebase = st->time_base;
        c->opaque = this;
        bool hw = false;
        if (wantHw && qEnvironmentVariableIsEmpty("WMP_NO_HWDEC") && hwWorthIt(c->codec_id) && preferredHwType() != AV_HWDEVICE_TYPE_NONE) {
            if (!hwDevice)
                av_hwdevice_ctx_create(&hwDevice, preferredHwType(), nullptr, nullptr, 0);
            if (hwDevice) {
                c->hw_device_ctx = av_buffer_ref(hwDevice);
                c->get_format = pickHw;
                hw = true;
            }
        }
        if (st->codecpar->codec_type == AVMEDIA_TYPE_VIDEO) {
            // Software decoding uses the cores it can; a hardware decoder has its own.
            c->thread_count = hw ? 1 : std::min(QThread::idealThreadCount(), 8);
            c->thread_type = FF_THREAD_FRAME | FF_THREAD_SLICE;
        }
        // The disc's sub-picture colours.
        if (c->codec_id == AV_CODEC_ID_DVD_SUBTITLE && spec.subPalette.size() == 16)
            av_opt_set(c->priv_data, "palette", spec.subPalette.join(',').toLatin1().constData(), 0);
        if (avcodec_open2(c, codec, nullptr) < 0) {
            avcodec_free_context(&c);
            return false;
        }
        *out = c;
        if (st->codecpar->codec_type == AVMEDIA_TYPE_VIDEO) {
            std::lock_guard<std::mutex> l(infoMu);
            stats.hwDecode = hw;
            stats.videoCodec = QString::fromLatin1(codec->name);
        } else if (st->codecpar->codec_type == AVMEDIA_TYPE_AUDIO) {
            std::lock_guard<std::mutex> l(infoMu);
            stats.audioCodec = QString::fromLatin1(codec->name);
        }
        return true;
    }

    static QString tagOf(AVStream *st, const char *key)
    {
        const AVDictionaryEntry *e = av_dict_get(st->metadata, key, nullptr, 0);
        return e ? QString::fromUtf8(e->value) : QString();
    }

    void listTracks()
    {
        QVector<TrackInfo> a, s;
        for (unsigned i = 0; i < fmt->nb_streams; i++) {
            AVStream *st = fmt->streams[i];
            if (st->disposition & AV_DISPOSITION_ATTACHED_PIC)
                continue;
            TrackInfo t;
            t.index = int(i);
            t.language = tagOf(st, "language");
            t.title = tagOf(st, "title");
            t.isDefault = st->disposition & AV_DISPOSITION_DEFAULT;
            t.forced = st->disposition & AV_DISPOSITION_FORCED;
            const AVCodecDescriptor *d = avcodec_descriptor_get(st->codecpar->codec_id);
            t.codec = d ? QString::fromLatin1(d->name) : QString();
            if (st->codecpar->codec_type == AVMEDIA_TYPE_AUDIO) {
                t.channels = st->codecpar->ch_layout.nb_channels;
                a << t;
            } else if (st->codecpar->codec_type == AVMEDIA_TYPE_SUBTITLE) {
                s << t;
            }
        }
        std::lock_guard<std::mutex> l(infoMu);
        audioTracks = a;
        subTracks = s + subTracks; // sidecars were added first, and stay last
    }

    // Subtitle files next to a video: same name, .srt, .ass or .vtt.
    void loadSidecars()
    {
        const QFileInfo fi(spec.filePath);
        const QRegularExpression names(QStringLiteral("^%1(\\.[A-Za-z-]{2,8})?\\.(srt|ass|ssa|vtt)$").arg(QRegularExpression::escape(fi.completeBaseName())), QRegularExpression::CaseInsensitiveOption);
        const auto entries = fi.dir().entryInfoList({QStringLiteral("*.srt"), QStringLiteral("*.ass"), QStringLiteral("*.ssa"), QStringLiteral("*.vtt")}, QDir::Files);
        for (const QFileInfo &e : entries) {
            auto m = names.match(e.fileName());
            if (!m.hasMatch() || e.size() > kMaxSidecarBytes)
                continue;
            QVector<SubtitleCue> cuesOut = readSidecar(e.absoluteFilePath());
            if (cuesOut.isEmpty())
                continue;
            TrackInfo t;
            t.index = sidecars.size();
            t.external = true;
            t.language = m.captured(1).mid(1);
            t.title = e.fileName();
            sidecars << cuesOut;
            std::lock_guard<std::mutex> l(infoMu);
            subTracks << t;
        }
    }

    QVector<SubtitleCue> readSidecar(const QString &path)
    {
        QVector<SubtitleCue> out;
        AVFormatContext *f = avformat_alloc_context();
        if (!f)
            return out;
        f->format_whitelist = av_strdup(kSubFormats);
        f->protocol_whitelist = av_strdup("file");
        if (avformat_open_input(&f, path.toUtf8().constData(), nullptr, nullptr) < 0)
            return out;
        AVCodecContext *c = nullptr;
        if (avformat_find_stream_info(f, nullptr) >= 0 && f->nb_streams > 0 && openDecoder(f->streams[0], &c, false)) {
            AVPacket *pkt = av_packet_alloc();
            while (!stop.load() && out.size() < int(kMaxCues) && av_read_frame(f, pkt) >= 0) {
                AVSubtitle sub{};
                int got = 0;
                if (pkt->stream_index == 0 && avcodec_decode_subtitle2(c, &sub, &got, pkt) >= 0 && got) {
                    const double tb = av_q2d(f->streams[0]->time_base);
                    const double start = pkt->pts == AV_NOPTS_VALUE ? 0 : double(pkt->pts) * tb;
                    const double end = start + (pkt->duration > 0 ? double(pkt->duration) * tb : 4.0);
                    auto cue = cueFrom(sub, start, end, 0, 0);
                    if (cue.has_value())
                        out << *cue;
                    avsubtitle_free(&sub);
                }
                av_packet_unref(pkt);
            }
            av_packet_free(&pkt);
        }
        avcodec_free_context(&c);
        avformat_close_input(&f);
        return out;
    }

    // Plain words from an ASS event line: dialogue fields and override tags off.
    static QString assText(const char *line)
    {
        QString s = QString::fromUtf8(line);
        // ReadOrder,Layer,Style,Name,MarginL,MarginR,MarginV,Effect,Text
        int commas = 0, i = 0;
        for (; i < s.size() && commas < 8; i++)
            if (s[i] == QLatin1Char(','))
                commas++;
        if (commas == 8)
            s = s.mid(i);
        static const QRegularExpression tags(QStringLiteral("\\{[^}]*\\}"));
        s.remove(tags);
        s.replace(QLatin1String("\\N"), QLatin1String("\n"));
        s.replace(QLatin1String("\\n"), QLatin1String("\n"));
        s.replace(QLatin1String("\\h"), QLatin1String(" "));
        return s.trimmed();
    }

    std::optional<SubtitleCue> cueFrom(const AVSubtitle &sub, double start, double end, int frameW, int frameH)
    {
        SubtitleCue cue;
        cue.start = start + double(sub.start_display_time) / 1000.0;
        cue.end = end;
        for (unsigned r = 0; r < sub.num_rects; r++) {
            const AVSubtitleRect *rc = sub.rects[r];
            cue.forced = cue.forced || (rc->flags & AV_SUBTITLE_FLAG_FORCED);
            if (rc->type == SUBTITLE_ASS && rc->ass) {
                if (!cue.text.isEmpty())
                    cue.text += QLatin1Char('\n');
                cue.text += assText(rc->ass);
            } else if (rc->type == SUBTITLE_TEXT && rc->text) {
                if (!cue.text.isEmpty())
                    cue.text += QLatin1Char('\n');
                cue.text += QString::fromUtf8(rc->text);
            } else if (rc->type == SUBTITLE_BITMAP && rc->w > 0 && rc->h > 0 && rc->data[0] && rc->data[1] && frameW > 0 && frameH > 0) {
                QImage img(rc->w, rc->h, QImage::Format_ARGB32);
                const auto *pal = reinterpret_cast<const uint32_t *>(rc->data[1]);
                for (int y = 0; y < rc->h; y++) {
                    auto *dst = reinterpret_cast<uint32_t *>(img.scanLine(y));
                    const uint8_t *src = rc->data[0] + y * rc->linesize[0];
                    for (int x = 0; x < rc->w; x++)
                        dst[x] = pal[src[x]];
                }
                cue.image = img;
                cue.rect = QRectF(double(rc->x) / frameW, double(rc->y) / frameH, double(rc->w) / frameW, double(rc->h) / frameH);
            }
        }
        if (cue.image.isNull() && cue.text.isEmpty())
            return std::nullopt;
        cue.id = nextCue++;
        return cue;
    }

    bool openFile()
    {
        fmt = avformat_alloc_context();
        fmt->format_whitelist = av_strdup(kFormats);
        fmt->protocol_whitelist = av_strdup("file");
        const QString native = spec.filePath;
        int rc;
        if (spec.cd) {
            auto stream = std::make_unique<CdStream>(spec.filePath);
            QString why;
            if (!stream->begin(&why)) {
                fail(why);
                return false;
            }
            reader = std::move(stream);
            fmt->pb = reader->avio();
            fmt->flags |= AVFMT_FLAG_CUSTOM_IO;
            rc = avformat_open_input(&fmt, nullptr, av_find_input_format("wav"), nullptr);
            if (rc < 0) {
                fail(QStringLiteral("This CD couldn't be read."));
                return false;
            }
        } else {
            rc = avformat_open_input(&fmt, native.toUtf8().constData(), nullptr, nullptr);
            if (rc < 0) {
                fail(QFile::exists(spec.filePath) ? QStringLiteral("WittyMediaPlayer can't read this kind of file.")
                                                  : QStringLiteral("This file can't be found."));
                return false;
            }
        }
        if (avformat_find_stream_info(fmt, nullptr) < 0) {
            fail(QStringLiteral("This file is damaged or isn't a media file."));
            return false;
        }
        if (!spec.cd)
            loadSidecars();
        listTracks();
        // The stream the file itself prefers, and no more decoders than needed.
        vIndex = av_find_best_stream(fmt, AVMEDIA_TYPE_VIDEO, -1, -1, nullptr, 0);
        aIndex = av_find_best_stream(fmt, AVMEDIA_TYPE_AUDIO, -1, -1, nullptr, 0);
        // Cover art isn't a video to decode: Qt reads the picture.
        for (unsigned i = 0; i < fmt->nb_streams; i++) {
            AVStream *st = fmt->streams[i];
            if (st->disposition & AV_DISPOSITION_ATTACHED_PIC) {
                if (int(i) == vIndex)
                    vIndex = -1;
                QImage art = QImage::fromData(st->attached_pic.data, st->attached_pic.size);
                if (!art.isNull()) {
                    std::lock_guard<std::mutex> l(infoMu);
                    cover = art;
                }
            }
        }
        if (vIndex < 0)
            vIndex = -1;
        if (aIndex < 0)
            aIndex = -1;
        vStream.store(vIndex >= 0 ? fmt->streams[vIndex] : nullptr);
        aStream.store(aIndex >= 0 ? fmt->streams[aIndex] : nullptr);
        for (unsigned i = 0; i < fmt->nb_streams; i++) {
            if (int(i) != vIndex && int(i) != aIndex)
                fmt->streams[i]->discard = AVDISCARD_ALL;
        }
        if (vIndex < 0 && aIndex < 0) {
            fail(QStringLiteral("This file has no picture or sound that WittyMediaPlayer can play."));
            return false;
        }
        const bool needVideo = vIndex >= 0, needAudio = aIndex >= 0;
        if (vIndex >= 0 && !openDecoder(fmt->streams[vIndex], &vctx, true)) {
            vIndex = -1;
        }
        if (aIndex >= 0 && !openDecoder(fmt->streams[aIndex], &actx, false)) {
            aIndex = -1;
        }
        if (vIndex < 0 && aIndex < 0) {
            fail(needVideo || needAudio ? QStringLiteral("This file uses a format WittyMediaPlayer can't decode.") : QStringLiteral("This file has nothing to play."));
            return false;
        }
        double d = fmt->duration > 0 ? double(fmt->duration) / AV_TIME_BASE : 0;
        durationSec.store(d);
        {
            std::lock_guard<std::mutex> l(infoMu);
            audioOnly = vIndex < 0;
            currentAudio = aIndex;
            const AVDictionaryEntry *e = av_dict_get(fmt->metadata, "title", nullptr, 0);
            title = e ? QString::fromUtf8(e->value) : QString();
            // Forced subtitle tracks show by themselves, as the file asks.
            for (const TrackInfo &t : subTracks)
                if (t.forced && !t.external) {
                    currentSub = t.index;
                    break;
                }
        }
        if (currentSub >= 0) {
            sIndex = currentSub;
            fmt->streams[sIndex]->discard = AVDISCARD_DEFAULT;
            if (!openDecoder(fmt->streams[sIndex], &sctx, false))
                sIndex = -1;
        }
        return true;
    }

    bool openDisc()
    {
        reader = std::make_unique<ExtentReader>(spec.files, spec.extents);
        fmt = avformat_alloc_context();
        fmt->pb = reader->avio();
        fmt->flags |= AVFMT_FLAG_CUSTOM_IO;
        fmt->correct_ts_overflow = 0; // this class keeps the timeline: a disc's clock restarts
        fmt->protocol_whitelist = av_strdup("none");
        fmt->probesize = 2 * 1024 * 1024;
        fmt->max_analyze_duration = 0;
        const AVInputFormat *in = av_find_input_format(spec.container == QLatin1String("mpegts") ? "mpegts" : "mpeg");
        if (!in || avformat_open_input(&fmt, nullptr, in, nullptr) < 0) {
            fail(QStringLiteral("This part of the disc couldn't be read."));
            return false;
        }
        durationSec.store(spec.durationSec);
        int64_t at = 0;
        for (const Segment &g : spec.segments) {
            segStart.push_back(at);
            at += g.bytes;
            if (g.basePts >= 0) {
                segOffset.push_back(g.startSec - double(g.basePts) / 90000.0);
                segKnown.push_back(1);
            } else {
                segOffset.push_back(0);
                segKnown.push_back(0);
            }
        }
        {
            std::lock_guard<std::mutex> l(infoMu);
            audioOnly = spec.videoId == 0;
        }
        return true;
    }

    int segmentOf(int64_t pos) const
    {
        auto it = std::upper_bound(segStart.begin(), segStart.end(), pos);
        return int(it - segStart.begin()) - 1;
    }

    // A timestamp from the disc, on the title's timeline. Each segment may restart
    // the clock; a segment continuing the last one keeps its offset, so joined cells
    // stay gapless.
    bool fixTimes(AVPacket *pkt, AVStream *st)
    {
        if (segStart.empty())
            return true;
        int64_t raw = pkt->pts != AV_NOPTS_VALUE ? pkt->pts : pkt->dts;
        if (raw == AV_NOPTS_VALUE)
            return true; // the decoder works these out
        const double tb = av_q2d(st->time_base);
        int k = pkt->pos >= 0 ? segmentOf(pkt->pos) : curSeg;
        if (k < 0)
            k = std::max(curSeg, 0);
        if (k >= int(segOffset.size()))
            k = int(segOffset.size()) - 1;
        double t = double(raw) * tb;
        if (k != curSeg) {
            curSeg = k;
            if (!segKnown[size_t(k)]) {
                const double startSec = spec.segments[k].startSec;
                if (haveLastOffset && std::abs(t + lastOffset - startSec) < 1.0) {
                    segOffset[size_t(k)] = lastOffset;
                } else {
                    segOffset[size_t(k)] = startSec - t;
                }
                segKnown[size_t(k)] = 1;
            }
        }
        double off = segOffset[size_t(k)];
        double sec = t + off;
        // A clock jump inside a segment: stay where the timeline was.
        if (lastDemuxSec >= 0 && std::abs(sec - lastDemuxSec) > 10.0) {
            off = lastDemuxSec - t;
            segOffset[size_t(k)] = off;
            sec = lastDemuxSec;
        }
        lastOffset = off;
        haveLastOffset = true;
        lastDemuxSec = sec;
        const int64_t shift = int64_t(std::llround(off / tb));
        if (pkt->pts != AV_NOPTS_VALUE)
            pkt->pts += shift;
        if (pkt->dts != AV_NOPTS_VALUE)
            pkt->dts += shift;
        return true;
    }

    // ------------------------------------------------------------------ demux

    void applyDiscSeek(int64_t offset, double at, int64_t pts)
    {
        const double tb = 1.0 / 90000.0;
        int k = segmentOf(offset);
        if (k >= 0 && pts >= 0) {
            segOffset[size_t(k)] = at - double(pts) * tb;
            segKnown[size_t(k)] = 1;
        }
        curSeg = -1;
        haveLastOffset = false;
        lastDemuxSec = -1;
        av_seek_frame(fmt, -1, offset, AVSEEK_FLAG_BYTE);
    }

    void flushAll(double target)
    {
        vq.clear();
        aq.clear();
        const uint64_t gen = ++seekGen;
        audioGen.store(gen);
        audio->clear(gen);
        audioStarted.store(false);
        dropVideoUntil.store(target);
        dropAudioUntil.store(target);
        videoDone.store(vIndex < 0);
        audioDone.store(aIndex < 0);
        endedSent.store(false);
        eof.store(false);
        setWall(target);
        if (vIndex >= 0)
            vq.push({ItemKind::Flush, nullptr, nullptr});
        if (aIndex >= 0)
            aq.push({ItemKind::Flush, nullptr, nullptr});
        std::lock_guard<std::mutex> l(cueMu);
        cues.clear();
    }

    void handleRequests()
    {
        bool seekNow = false, byOffset = false, restart = false;
        double sec = 0, at = 0;
        int64_t off = 0, pts = -1;
        int wa = -2, ws = -2;
        bool wsExt = false;
        {
            std::lock_guard<std::mutex> l(reqMu);
            seekNow = seekWanted;
            byOffset = seekByOffset;
            sec = seekSec;
            off = seekOffset;
            at = seekAt;
            pts = seekPts;
            restart = restartWanted;
            wa = wantAudio;
            ws = wantSub;
            wsExt = wantSubExternal;
            seekWanted = restartWanted = false;
            wantAudio = wantSub = -2;
        }
        if (ws != -2)
            switchSubtitle(ws, wsExt);
        if (wa != -2 && wa != aIndex) {
            switchAudio(wa);
            if (!seekNow) {
                seekNow = true;
                byOffset = false;
                sec = position.load();
            }
        }
        if (discSubChanged.exchange(false))
            updateDiscSubtitle();
        if (restart) {
            if (spec.isDisc()) {
                seekNow = byOffset = true;
                off = 0;
                at = 0;
                pts = -1;
                sec = 0;
            } else {
                seekNow = true;
                byOffset = false;
                sec = 0;
            }
        }
        if (!seekNow)
            return;
        flushAll(sec);
        if (byOffset) {
            applyDiscSeek(off, at, pts);
        } else if (fmt) {
            int64_t ts = int64_t(sec * AV_TIME_BASE);
            if (fmt->start_time != AV_NOPTS_VALUE)
                ts += fmt->start_time;
            avformat_seek_file(fmt, -1, INT64_MIN, ts, ts, 0);
        }
    }

    void switchAudio(int index)
    {
        AVCodecContext *next = nullptr;
        if (index >= 0 && index < int(fmt->nb_streams) && !openDecoder(fmt->streams[index], &next, false))
            return;
        if (aIndex >= 0)
            fmt->streams[aIndex]->discard = AVDISCARD_ALL;
        if (index >= 0)
            fmt->streams[index]->discard = AVDISCARD_DEFAULT;
        aq.clear();
        aq.push({ItemKind::Swap, nullptr, next});
        aIndex = index;
        aStream.store(index >= 0 ? fmt->streams[index] : nullptr);
        {
            std::lock_guard<std::mutex> l(infoMu);
            currentAudio = index;
        }
    }

    void switchSubtitle(int index, bool external)
    {
        if (!spec.isDisc()) {
            if (sIndex >= 0)
                fmt->streams[sIndex]->discard = AVDISCARD_ALL;
            sIndex = -1;
            avcodec_free_context(&sctx);
            {
                std::lock_guard<std::mutex> c(cueMu);
                cues.clear();
                if (external && index >= 0 && index < sidecars.size())
                    for (const SubtitleCue &cue : sidecars[index])
                        cues.push_back(cue);
            }
            if (!external && index >= 0 && index < int(fmt->nb_streams)) {
                fmt->streams[index]->discard = AVDISCARD_DEFAULT;
                if (openDecoder(fmt->streams[index], &sctx, false))
                    sIndex = index;
                // Back a little: this stream's lines were being skipped until now.
                if (sIndex >= 0) {
                    std::lock_guard<std::mutex> l(reqMu);
                    if (!seekWanted) {
                        seekWanted = true;
                        seekByOffset = false;
                        seekSec = position.load();
                    }
                }
            }
            std::lock_guard<std::mutex> l(infoMu);
            currentSub = index;
            currentSubExternal = external;
        }
    }

    void updateDiscSubtitle()
    {
        const int id = discSubId.load();
        for (unsigned i = 0; i < fmt->nb_streams; i++) {
            AVStream *st = fmt->streams[i];
            if (st->codecpar->codec_type == AVMEDIA_TYPE_SUBTITLE || st->codecpar->codec_type == AVMEDIA_TYPE_UNKNOWN)
                st->discard = (id != 0 && st->id == id) ? AVDISCARD_DEFAULT : AVDISCARD_ALL;
        }
        if (sIndex >= 0 && fmt->streams[sIndex]->id != id) {
            sIndex = -1;
            avcodec_free_context(&sctx);
        }
        std::lock_guard<std::mutex> c(cueMu);
        cues.clear();
    }

    // Disc streams appear as they are met; choose and open them on first sight.
    void considerStream(AVStream *st)
    {
        const AVMediaType type = st->codecpar->codec_type;
        const int idx = st->index;
        if (idx == vIndex || idx == aIndex || idx == sIndex)
            return;
        if (type == AVMEDIA_TYPE_VIDEO && spec.videoId != 0 && st->id == spec.videoId && vIndex < 0) {
            if (openDecoder(st, &vctx, true)) {
                vStream.store(st);
                vIndex = idx;
            } else
                fail(QStringLiteral("This disc's video format can't be decoded."));
            return;
        }
        if (type == AVMEDIA_TYPE_AUDIO && aIndex < 0 && ((spec.audioId != 0 && st->id == spec.audioId) || spec.audioFirst)) {
            if (openDecoder(st, &actx, false)) {
                aq.push({ItemKind::Swap, nullptr, actx});
                actx = nullptr;
                aStream.store(st);
                aIndex = idx;
                audioDone.store(false);
            }
            return;
        }
        if (type == AVMEDIA_TYPE_SUBTITLE && discSubId.load() != 0 && st->id == discSubId.load() && sIndex < 0) {
            if (openDecoder(st, &sctx, false))
                sIndex = idx;
            return;
        }
        st->discard = AVDISCARD_ALL;
    }

    void decodeSubtitle(AVPacket *pkt)
    {
        if (!sctx)
            return;
        AVSubtitle sub{};
        int got = 0;
        if (avcodec_decode_subtitle2(sctx, &sub, &got, pkt) < 0 || !got)
            return;
        AVStream *st = fmt->streams[pkt->stream_index];
        const double tb = av_q2d(st->time_base);
        double start = pkt->pts != AV_NOPTS_VALUE ? double(pkt->pts) * tb : position.load();
        if (!spec.isDisc() && fmt->start_time != AV_NOPTS_VALUE)
            start -= double(fmt->start_time) / AV_TIME_BASE;
        double end = 1e12;
        if (sub.end_display_time != UINT32_MAX && sub.end_display_time > 0)
            end = start + double(sub.end_display_time) / 1000.0;
        else if (pkt->duration > 0)
            end = start + double(pkt->duration) * tb;
        int w = sctx->width, h = sctx->height;
        if (w <= 0 || h <= 0) {
            std::lock_guard<std::mutex> l(infoMu);
            w = stats.width;
            h = stats.height;
        }
        auto cue = cueFrom(sub, start, end, w, h);
        avsubtitle_free(&sub);
        std::lock_guard<std::mutex> l(cueMu);
        // A new picture replaces the one before; an empty one only clears it.
        for (auto &c : cues)
            if (c.end > start && c.start < start && c.end >= 1e11)
                c.end = start;
        if (cue.has_value())
            cues.push_back(*cue);
        const double before = position.load() - 30;
        while (!cues.empty() && (cues.front().end < before || cues.size() > kMaxCues))
            cues.pop_front();
    }

    void demuxLoop()
    {
        bool ok = spec.isDisc() ? openDisc() : openFile();
        if (!ok) {
            stop.store(true);
            vq.wake();
            aq.wake();
            return;
        }
        videoDone.store(spec.isDisc() ? spec.videoId == 0 : vIndex < 0);
        audioDone.store(spec.isDisc() ? (spec.audioId == 0 && !spec.audioFirst) : aIndex < 0);
        if (stop.load())
            return;
        QMetaObject::invokeMethod(owner, [o = owner] {
            emit o->opened();
            emit o->tracksChanged();
        }, Qt::QueuedConnection);

        // Start in the middle when asked to.
        if (spec.isDisc() && spec.startOffset > 0) {
            dropVideoUntil.store(spec.startSec);
            dropAudioUntil.store(spec.startSec);
            setWall(spec.startSec);
            applyDiscSeek(spec.startOffset, spec.startAt, spec.startPts);
        } else if (spec.isDisc() && spec.startSec > 0) {
            dropVideoUntil.store(spec.startSec);
            dropAudioUntil.store(spec.startSec);
            setWall(spec.startSec);
        }

        videoThread = QThread::create([this] { videoLoop(); });
        videoThread->start();
        audioThread = QThread::create([this] { audioLoop(); });
        audioThread->start();

        // In a disc, the streams appear as they're found; tell the threads who's first.
        AVPacket *pkt = av_packet_alloc();
        while (!stop.load()) {
            handleRequests();
            // Hold back when both queues have enough; never when one has run dry,
            // or a badly interleaved file could stall itself.
            if (vq.bytes() > kMaxVideoQueue || aq.bytes() > kMaxAudioQueue) {
                const bool otherDry = (vIndex >= 0 && vq.empty()) || (aIndex >= 0 && aq.empty());
                if (!otherDry || vq.bytes() > 3 * kMaxVideoQueue) {
                    std::unique_lock<std::mutex> l(reqMu);
                    reqCv.wait_for(l, std::chrono::milliseconds(10));
                    continue;
                }
            }
            if (eof.load()) {
                std::unique_lock<std::mutex> l(reqMu);
                reqCv.wait_for(l, std::chrono::milliseconds(30));
                continue;
            }
            int rc = av_read_frame(fmt, pkt);
            if (rc < 0) {
                if (stop.load())
                    break;
                if (rc == AVERROR(EAGAIN)) {
                    continue;
                }
                eof.store(true);
                if (vIndex >= 0)
                    vq.push({ItemKind::Eof, nullptr, nullptr});
                else
                    videoDone.store(true);
                if (aIndex >= 0)
                    aq.push({ItemKind::Eof, nullptr, nullptr});
                else
                    audioDone.store(true);
                checkEnded();
                continue;
            }
            AVStream *st = fmt->streams[pkt->stream_index];
            if (spec.isDisc()) {
                considerStream(st);
                const int idx = pkt->stream_index;
                if (idx != vIndex && idx != aIndex && idx != sIndex) {
                    av_packet_unref(pkt);
                    continue;
                }
                fixTimes(pkt, st);
            }
            if (pkt->stream_index == vIndex) {
                vq.push({ItemKind::Packet, av_packet_clone(pkt), nullptr});
            } else if (pkt->stream_index == aIndex) {
                aq.push({ItemKind::Packet, av_packet_clone(pkt), nullptr});
            } else if (pkt->stream_index == sIndex) {
                decodeSubtitle(pkt);
            }
            av_packet_unref(pkt);
        }
        av_packet_free(&pkt);
    }

    // ------------------------------------------------------------------ video

    // The video thread's own: the deinterlacer and the converter.
    AVFilterGraph *graph = nullptr;
    AVFilterContext *srcFilter = nullptr, *sinkFilter = nullptr;
    int graphW = 0, graphH = 0, graphFmt = -1;
    SwsContext *sws = nullptr;
    int swsFmt = -1, swsW = 0, swsH = 0;
    bool wasLate = false;

    void freeGraph()
    {
        avfilter_graph_free(&graph);
        srcFilter = sinkFilter = nullptr;
        graphFmt = -1;
    }

    bool buildGraph(const AVFrame *f)
    {
        freeGraph();
        graph = avfilter_graph_alloc();
        const char *mode = QThread::idealThreadCount() >= 4 ? "bwdif" : "yadif";
        AVStream *st = vStream.load();
        AVRational tb = st ? st->time_base : AVRational{1, 90000};
        AVRational sar = f->sample_aspect_ratio.num ? f->sample_aspect_ratio : AVRational{1, 1};
        char args[256];
        std::snprintf(args, sizeof args, "video_size=%dx%d:pix_fmt=%d:time_base=%d/%d:pixel_aspect=%d/%d", f->width, f->height, f->format, tb.num, tb.den, sar.num, sar.den);
        if (avfilter_graph_create_filter(&srcFilter, avfilter_get_by_name("buffer"), "in", args, nullptr, graph) < 0 ||
            avfilter_graph_create_filter(&sinkFilter, avfilter_get_by_name("buffersink"), "out", nullptr, nullptr, graph) < 0) {
            freeGraph();
            return false;
        }
        AVFilterInOut *in = avfilter_inout_alloc(), *out = avfilter_inout_alloc();
        out->name = av_strdup("in");
        out->filter_ctx = srcFilter;
        in->name = av_strdup("out");
        in->filter_ctx = sinkFilter;
        char desc[128];
        // Only frames marked interlaced are touched, in the disc's own field order.
        std::snprintf(desc, sizeof desc, "format=yuv420p,%s=mode=send_frame:parity=auto:deint=interlaced", mode);
        int rc = avfilter_graph_parse_ptr(graph, desc, &in, &out, nullptr);
        if (rc >= 0)
            rc = avfilter_graph_config(graph, nullptr);
        avfilter_inout_free(&in);
        avfilter_inout_free(&out);
        if (rc < 0) {
            freeGraph();
            return false;
        }
        graphW = f->width;
        graphH = f->height;
        graphFmt = f->format;
        std::lock_guard<std::mutex> l(infoMu);
        stats.deinterlacing = true;
        return true;
    }

    // Formats the screen takes directly; anything else becomes one of them.
    static bool drawable(int f)
    {
        return f == AV_PIX_FMT_YUV420P || f == AV_PIX_FMT_YUVJ420P || f == AV_PIX_FMT_NV12 || f == AV_PIX_FMT_P010;
    }

    AVFrame *convert(AVFrame *f)
    {
        const AVPixFmtDescriptor *d = av_pix_fmt_desc_get(AVPixelFormat(f->format));
        const bool deep = d && d->comp[0].depth > 8;
        const AVPixelFormat target = deep ? AV_PIX_FMT_P010 : AV_PIX_FMT_YUV420P;
        if (!sws || swsFmt != f->format || swsW != f->width || swsH != f->height) {
            sws_freeContext(sws);
            sws = sws_getContext(f->width, f->height, AVPixelFormat(f->format), f->width, f->height, target, SWS_FAST_BILINEAR, nullptr, nullptr, nullptr);
            swsFmt = f->format;
            swsW = f->width;
            swsH = f->height;
        }
        if (!sws)
            return nullptr;
        AVFrame *o = av_frame_alloc();
        o->format = target;
        o->width = f->width;
        o->height = f->height;
        if (av_frame_get_buffer(o, 0) < 0) {
            av_frame_free(&o);
            return nullptr;
        }
        sws_scale(sws, f->data, f->linesize, 0, f->height, o->data, o->linesize);
        av_frame_copy_props(o, f);
        return o;
    }

    double frameTime(const AVFrame *f, const AVStream *st) const
    {
        int64_t ts = f->best_effort_timestamp;
        if (ts == AV_NOPTS_VALUE)
            ts = f->pts;
        if (ts == AV_NOPTS_VALUE)
            return -1;
        double t = double(ts) * av_q2d(st->time_base);
        if (!spec.isDisc() && fmt->start_time != AV_NOPTS_VALUE)
            t -= double(fmt->start_time) / AV_TIME_BASE;
        return t;
    }

    void showFrame(AVFrame *f, double t)
    {
        AVFrame *ready = nullptr;
        if (drawable(f->format)) {
            ready = av_frame_clone(f);
        } else {
            ready = convert(f);
        }
        if (!ready)
            return;
        // Pace it against the clock.
        if (t >= 0) {
            for (;;) {
                if (stop.load())
                    break;
                waitWhilePaused();
                if (dropVideoUntil.load() >= 0)
                    break;
                const double now = clock();
                if (now < 0) {
                    setWall(t);
                    break;
                }
                const double delay = t - now;
                if (delay <= 0.002)
                    break;
                // A long wait may be a clock change: look again soon.
                std::this_thread::sleep_for(std::chrono::duration<double>(std::min(delay, 0.05)));
            }
        }
        if (stop.load()) {
            av_frame_free(&ready);
            return;
        }
        if (t >= 0)
            position.store(t);
        if (sink)
            sink->presentFrame(shareFrame(ready));
        else
            av_frame_free(&ready);
        framesShown++;
        if (!gotFrame.exchange(true))
            QMetaObject::invokeMethod(owner, [o = owner] { emit o->firstFrame(); }, Qt::QueuedConnection);
    }

    void handleDecoded(AVFrame *f, AVStream *st)
    {
        framesDecoded++;
        {
            std::lock_guard<std::mutex> l(infoMu);
            stats.width = f->width;
            stats.height = f->height;
        }
        double t = frameTime(f, st);
        const double target = dropVideoUntil.load();
        if (target >= 0) {
            if (t >= 0 && t < target - 0.02)
                return; // decoding forward to the place asked for
            dropVideoUntil.store(-1);
            setWall(t >= 0 ? t : target);
        }
        // Behind: drop what can be dropped, and ask the decoder to skip work.
        if (t >= 0) {
            const double now = clock();
            if (now >= 0) {
                const double late = now - t;
                if (late > 0.08 && gotFrame.load()) {
                    framesDropped++;
                    if (late > 0.25 && vctx && vctx->skip_frame < AVDISCARD_NONREF) {
                        vctx->skip_frame = AVDISCARD_NONREF;
                        vctx->skip_loop_filter = AVDISCARD_ALL;
                        wasLate = true;
                    }
                    return;
                }
                if (wasLate && late < 0.04 && vctx) {
                    vctx->skip_frame = AVDISCARD_DEFAULT;
                    vctx->skip_loop_filter = AVDISCARD_DEFAULT;
                    wasLate = false;
                }
            }
        }
        const bool interlaced = f->flags & AV_FRAME_FLAG_INTERLACED;
        if ((interlaced || graph) && (spec.deinterlace || !spec.isDisc())) {
            if (!graph || graphW != f->width || graphH != f->height || graphFmt != f->format) {
                if (!buildGraph(f)) {
                    showFrame(f, t);
                    return;
                }
            }
            drainGraph(f, st);
            return;
        }
        showFrame(f, t);
    }

    void drainGraph(AVFrame *f, AVStream *st)
    {
        if (f->best_effort_timestamp != AV_NOPTS_VALUE)
            f->pts = f->best_effort_timestamp;
        if (av_buffersrc_add_frame_flags(srcFilter, f, AV_BUFFERSRC_FLAG_KEEP_REF) < 0)
            return;
        AVFrame *out = av_frame_alloc();
        while (!stop.load() && av_buffersink_get_frame(sinkFilter, out) >= 0) {
            showFrame(out, frameTime(out, st));
            av_frame_unref(out);
        }
        av_frame_free(&out);
    }

    void receiveVideo(AVFrame *frame, AVFrame *sw, AVStream *st)
    {
        while (!stop.load()) {
            int rc = avcodec_receive_frame(vctx, frame);
            if (rc < 0)
                break;
            AVFrame *use = frame;
            if (frame->hw_frames_ctx) {
                av_frame_unref(sw);
                if (av_hwframe_transfer_data(sw, frame, 0) < 0) {
                    av_frame_unref(frame);
                    continue;
                }
                av_frame_copy_props(sw, frame);
                // The graphics card's surfaces can be taller than the picture.
                sw->width = frame->width;
                sw->height = frame->height;
                use = sw;
            }
            handleDecoded(use, st);
            av_frame_unref(frame);
        }
    }

    void videoLoop()
    {
        AVFrame *frame = av_frame_alloc(), *sw = av_frame_alloc();
        Item it;
        while (vq.pop(&it, stop)) {
            waitWhilePaused();
            if (it.kind == ItemKind::Flush) {
                if (vctx)
                    avcodec_flush_buffers(vctx);
                freeGraph();
                continue;
            }
            if (vIndex < 0 || !vctx) {
                av_packet_free(&it.pkt);
                if (it.kind == ItemKind::Eof)
                    videoDone.store(true);
                continue;
            }
            AVStream *st = vStream.load();
            if (it.kind == ItemKind::Eof) {
                avcodec_send_packet(vctx, nullptr);
                receiveVideo(frame, sw, st);
                avcodec_flush_buffers(vctx);
                if (graph) {
                    av_buffersrc_add_frame_flags(srcFilter, nullptr, 0);
                    AVFrame *out = av_frame_alloc();
                    while (!stop.load() && av_buffersink_get_frame(sinkFilter, out) >= 0) {
                        showFrame(out, frameTime(out, st));
                        av_frame_unref(out);
                    }
                    av_frame_free(&out);
                    freeGraph();
                }
                videoDone.store(true);
                checkEnded();
                continue;
            }
            int rc = avcodec_send_packet(vctx, it.pkt);
            av_packet_free(&it.pkt);
            if (rc < 0 && rc != AVERROR(EAGAIN))
                continue; // a bad packet: the next keyframe recovers
            receiveVideo(frame, sw, st);
        }
        av_frame_free(&frame);
        av_frame_free(&sw);
        freeGraph();
        sws_freeContext(sws);
        sws = nullptr;
    }

    // ------------------------------------------------------------------ audio

    void audioLoop()
    {
        AVCodecContext *ctx = actx;
        actx = nullptr;
        SwrContext *swr = nullptr;
        int swrFmt = -1, swrRate = 0, swrCh = 0;
        AVFrame *frame = av_frame_alloc();
        std::vector<uint8_t> out;
        double nextPts = -1;
        const int outRate = audio->sampleRate();

        auto makeSwr = [&](const AVFrame *f) {
            swr_free(&swr);
            AVChannelLayout stereo = AV_CHANNEL_LAYOUT_STEREO;
            if (swr_alloc_set_opts2(&swr, &stereo, AV_SAMPLE_FMT_S16, outRate, &f->ch_layout, AVSampleFormat(f->format), f->sample_rate, 0, nullptr) < 0)
                return false;
            if (spec.frontOnly && f->ch_layout.nb_channels > 2) {
                // Karaoke, sing along: the backing track on the front channels only.
                std::vector<double> m(size_t(2 * f->ch_layout.nb_channels), 0.0);
                m[0] = 1.0;
                m[size_t(f->ch_layout.nb_channels) + 1] = 1.0;
                swr_set_matrix(swr, m.data(), f->ch_layout.nb_channels);
            }
            if (swr_init(swr) < 0)
                return false;
            swrFmt = f->format;
            swrRate = f->sample_rate;
            swrCh = f->ch_layout.nb_channels;
            return true;
        };

        Item it;
        while (aq.pop(&it, stop)) {
            waitWhilePaused();
            if (it.kind == ItemKind::Flush) {
                if (ctx)
                    avcodec_flush_buffers(ctx);
                if (swr) {
                    swr_free(&swr);
                    swrFmt = -1;
                }
                nextPts = -1;
                continue;
            }
            if (it.kind == ItemKind::Swap) {
                avcodec_free_context(&ctx);
                ctx = it.newCtx;
                swr_free(&swr);
                swrFmt = -1;
                nextPts = -1;
                continue;
            }
            if (it.kind == ItemKind::Eof) {
                if (ctx) {
                    avcodec_send_packet(ctx, nullptr);
                    while (avcodec_receive_frame(ctx, frame) >= 0)
                        av_frame_unref(frame);
                    avcodec_flush_buffers(ctx);
                }
                // Let what's queued play out before calling it done.
                while (!stop.load() && audio->queuedSec() > 0.01 && audioGenSame())
                    std::this_thread::sleep_for(std::chrono::milliseconds(15));
                if (audioGenSame()) {
                    const double last = audio->positionSec();
                    if (last >= 0)
                        setWall(last);
                    audioDone.store(true);
                    checkEnded();
                }
                continue;
            }
            if (!ctx || aIndex < 0) {
                av_packet_free(&it.pkt);
                continue;
            }
            const uint64_t gen = audioGen.load();
            AVStream *st = aStream.load();
            int rc = avcodec_send_packet(ctx, it.pkt);
            av_packet_free(&it.pkt);
            if (rc < 0)
                continue;
            while (!stop.load() && avcodec_receive_frame(ctx, frame) >= 0) {
                double t = -1;
                int64_t ts = frame->best_effort_timestamp;
                if (ts == AV_NOPTS_VALUE)
                    ts = frame->pts;
                if (ts != AV_NOPTS_VALUE) {
                    t = double(ts) * av_q2d(st->time_base);
                    if (!spec.isDisc() && fmt->start_time != AV_NOPTS_VALUE)
                        t -= double(fmt->start_time) / AV_TIME_BASE;
                } else if (nextPts >= 0) {
                    t = nextPts;
                }
                const double frameDur = frame->sample_rate > 0 ? double(frame->nb_samples) / frame->sample_rate : 0;
                if (t >= 0)
                    nextPts = t + frameDur;
                // Decoding forward to the place asked for.
                const double target = dropAudioUntil.load();
                if (target >= 0) {
                    if (t >= 0 && t + frameDur < target) {
                        av_frame_unref(frame);
                        continue;
                    }
                    dropAudioUntil.store(-1);
                }
                if (!swr || swrFmt != frame->format || swrRate != frame->sample_rate || swrCh != frame->ch_layout.nb_channels) {
                    if (!makeSwr(frame)) {
                        av_frame_unref(frame);
                        continue;
                    }
                }
                const int maxOut = swr_get_out_samples(swr, frame->nb_samples);
                out.resize(size_t(maxOut) * AudioOutput::kBytesPerFrame);
                uint8_t *dst = out.data();
                int got = swr_convert(swr, &dst, maxOut, const_cast<const uint8_t **>(frame->extended_data), frame->nb_samples);
                av_frame_unref(frame);
                if (got <= 0)
                    continue;
                audioStarted.store(true);
                if (!audio->write(reinterpret_cast<const char *>(dst), qsizetype(got) * AudioOutput::kBytesPerFrame, t < 0 ? 0 : t, gen, &stop))
                    break;
            }
        }
        avcodec_free_context(&ctx);
        swr_free(&swr);
        av_frame_free(&frame);
    }

    // A seek clears the end of the stream.
    bool audioGenSame() const { return eof.load(); }

    void checkEnded()
    {
        if (videoDone.load() && audioDone.load() && eof.load() && !endedSent.exchange(true)) {
            QMetaObject::invokeMethod(owner, [o = owner] { emit o->ended(); }, Qt::QueuedConnection);
        }
    }

    void wakeAll()
    {
        vq.wake();
        aq.wake();
        {
            std::lock_guard<std::mutex> l(pauseMu);
            pauseCv.notify_all();
        }
        audio->wake();
        std::lock_guard<std::mutex> l(reqMu);
        reqCv.notify_all();
    }

    void shutdown()
    {
        stop.store(true);
        if (reader)
            reader->stop();
        wakeAll();
        const auto finish = [this](QThread *&t) {
            if (t) {
                while (!t->wait(50))
                    wakeAll();
                delete t;
                t = nullptr;
            }
        };
        finish(demuxThread);
        wakeAll();
        finish(videoThread);
        finish(audioThread);
    }
};

namespace {
enum AVPixelFormat pickHw(AVCodecContext *, const enum AVPixelFormat *fmts)
{
    const enum AVPixelFormat *first = fmts;
    for (; *fmts != AV_PIX_FMT_NONE; fmts++) {
        if (*fmts == AV_PIX_FMT_D3D11)
            return *fmts;
    }
    // This picture isn't one the graphics card takes: the software path.
    for (fmts = first; *fmts != AV_PIX_FMT_NONE; fmts++) {
        const AVPixFmtDescriptor *d = av_pix_fmt_desc_get(*fmts);
        if (d && !(d->flags & AV_PIX_FMT_FLAG_HWACCEL))
            return *fmts;
    }
    return *first;
}
} // namespace

// ---------------------------------------------------------------------- player

Player::Player(QObject *parent) : QObject(parent)
{
    qRegisterMetaType<FramePtr>();
    audio_ = new AudioOutput();
    watch_.setInterval(500);
    connect(&watch_, &QTimer::timeout, this, [this] {
        if (vlcActive_) {
            if (const int64_t bad = vlc_->stats().badBlocks; bad > reportedBad_) {
                reportedBad_ = bad;
                emit discTrouble(QStringLiteral("Part of this disc couldn't be read, so it was skipped. The disc may be dirty or scratched."));
            }
            return;
        }
        if (!session_ || !session_->reader)
            return;
        const int64_t bad = session_->reader->badBlocks();
        if (bad > reportedBad_) {
            reportedBad_ = bad;
            emit discTrouble(QStringLiteral("Part of this disc couldn't be read, so it was skipped. The disc may be dirty or scratched."));
        }
        if (session_->reader->discLost() && !reportedLost_) {
            reportedLost_ = true;
            emit discLost();
        }
    });
    watch_.start();
}

Player::~Player()
{
    stop();
    // A drive that won't answer gets a moment, not forever: leaving ends it anyway.
    for (int i = 0; i < 200 && retiring_.load() > 0; i++)
        std::this_thread::sleep_for(std::chrono::milliseconds(10));
    audio_->stop();
    delete audio_;
}

void Player::initAudio(bool noDevice)
{
    audio_->start(noDevice);
    audio_->setVolume(volume_);
}

void Player::retire(std::shared_ptr<Session> s)
{
    if (!s)
        return;
    // A drive stuck on a bad sector mustn't hold up the window.
    s->stop.store(true);
    if (s->reader)
        s->reader->stop();
    retiring_.fetch_add(1);
    QThread *t = QThread::create([s, this] {
        s->shutdown();
        retiring_.fetch_sub(1);
    });
    connect(t, &QThread::finished, t, &QObject::deleteLater);
    t->start();
}

void Player::open(const MediaSpec &spec)
{
    if (vlc_)
        vlc_->stop();
    vlcActive_ = useVlc_ && vlc_ && vlc_->loaded() && !spec.cd;
    if (vlcActive_) {
        auto previous = session_;
        session_.reset();
        retire(previous);
        audio_->clear(generation_ += uint64_t(1) << 32);
        vlc_->setVolume(volume_);
        vlc_->open(spec, sink_);
        vlc_->setPaused(paused_);
        return;
    }
    auto previous = session_;
    session_.reset();
    retire(previous);
    // Far enough on that nothing a finished session counted can meet it.
    generation_ += uint64_t(1) << 32;
    audio_->clear(generation_);
    reportedBad_ = 0;
    reportedLost_ = false;
    auto s = std::make_shared<Session>();
    s->owner = this;
    s->sink = sink_;
    s->audio = audio_;
    s->spec = spec;
    s->seekGen = generation_;
    s->audioGen.store(generation_);
    s->paused.store(paused_);
    s->pauseWall(paused_);
    session_ = s;
    s->demuxThread = QThread::create([p = s.get()] {
        p->demuxLoop();
        if (p->reader)
            p->reader->finishedReading();
    });
    s->demuxThread->start();
}

void Player::stop()
{
    if (vlc_)
        vlc_->stop();
    vlcActive_ = false;
    auto s = session_;
    session_.reset();
    if (s) {
        s->shutdown();
        generation_ += uint64_t(1) << 32;
        audio_->clear(generation_);
    }
}

bool Player::hasMedia() const
{
    return vlcActive_ ? vlc_->hasMedia() : session_ != nullptr;
}

bool Player::enableVlc(const QString &folder, QString *why)
{
    if (!vlc_)
        vlc_ = new VlcPlayback(this);
    return vlc_->load(folder, why);
}

QString Player::vlcVersion() const
{
    return vlc_ ? vlc_->version() : QString();
}

void Player::setPaused(bool paused)
{
    paused_ = paused;
    if (vlcActive_) {
        vlc_->setPaused(paused);
        return;
    }
    audio_->setPaused(paused);
    if (auto s = session_) {
        {
            std::lock_guard<std::mutex> l(s->pauseMu);
            s->paused.store(paused);
            s->pauseCv.notify_all();
        }
        s->pauseWall(paused);
    }
}

void Player::seek(double sec)
{
    if (vlcActive_) {
        vlc_->seek(sec);
        return;
    }
    auto s = session_;
    if (!s)
        return;
    double d = s->durationSec.load();
    if (d > 0)
        sec = std::clamp(sec, 0.0, d);
    std::lock_guard<std::mutex> l(s->reqMu);
    s->seekWanted = true;
    s->seekByOffset = false;
    s->seekSec = std::max(sec, 0.0);
    s->reqCv.notify_all();
}

void Player::seekDisc(int64_t offset, double at, int64_t pts, double target)
{
    if (vlcActive_) {
        vlc_->seekDisc(offset, target);
        return;
    }
    auto s = session_;
    if (!s)
        return;
    std::lock_guard<std::mutex> l(s->reqMu);
    s->seekWanted = true;
    s->seekByOffset = true;
    s->seekOffset = offset;
    s->seekAt = at;
    s->seekPts = pts;
    s->seekSec = target;
    s->reqCv.notify_all();
}

void Player::restart()
{
    if (vlcActive_) {
        vlc_->restart();
        return;
    }
    auto s = session_;
    if (!s)
        return;
    std::lock_guard<std::mutex> l(s->reqMu);
    s->restartWanted = true;
    s->reqCv.notify_all();
}

void Player::setVolume(double v)
{
    volume_ = v;
    audio_->setVolume(v);
    if (vlc_)
        vlc_->setVolume(v);
}

void Player::setAudioTrack(int index)
{
    if (vlcActive_) {
        vlc_->setAudioTrack(index);
        return;
    }
    auto s = session_;
    if (!s)
        return;
    std::lock_guard<std::mutex> l(s->reqMu);
    s->wantAudio = index;
    s->reqCv.notify_all();
}

void Player::setSubtitleTrack(int index, bool external)
{
    if (vlcActive_) {
        vlc_->setSubtitleTrack(index);
        return;
    }
    auto s = session_;
    if (!s)
        return;
    std::lock_guard<std::mutex> l(s->reqMu);
    s->wantSub = index;
    s->wantSubExternal = external;
    s->reqCv.notify_all();
}

void Player::setDiscSubtitle(int id, bool forcedOnly)
{
    if (vlcActive_) {
        vlc_->setDiscSubtitle(id, forcedOnly);
        return;
    }
    auto s = session_;
    if (!s)
        return;
    s->forcedOnly.store(forcedOnly);
    if (s->discSubId.exchange(id) != id)
        s->discSubChanged.store(true);
    s->reqCv.notify_all();
}

QVector<TrackInfo> Player::audioTracks() const
{
    if (vlcActive_)
        return vlc_->audioTracks();
    auto s = session_;
    if (!s)
        return {};
    std::lock_guard<std::mutex> l(s->infoMu);
    return s->audioTracks;
}

QVector<TrackInfo> Player::subtitleTracks() const
{
    if (vlcActive_)
        return vlc_->subtitleTracks();
    auto s = session_;
    if (!s)
        return {};
    std::lock_guard<std::mutex> l(s->infoMu);
    return s->subTracks;
}

int Player::currentAudioTrack() const
{
    if (vlcActive_)
        return vlc_->currentAudioTrack();
    auto s = session_;
    if (!s)
        return -1;
    std::lock_guard<std::mutex> l(s->infoMu);
    return s->currentAudio;
}

int Player::currentSubtitleTrack() const
{
    if (vlcActive_)
        return vlc_->currentSubtitleTrack();
    auto s = session_;
    if (!s)
        return -1;
    std::lock_guard<std::mutex> l(s->infoMu);
    return s->currentSub;
}

bool Player::currentSubtitleExternal() const
{
    if (vlcActive_)
        return false;
    auto s = session_;
    if (!s)
        return false;
    std::lock_guard<std::mutex> l(s->infoMu);
    return s->currentSubExternal;
}

double Player::position() const
{
    if (vlcActive_)
        return vlc_->position();
    auto s = session_;
    if (!s)
        return 0;
    // With no picture, the sound says where we are.
    if (s->audioOnlyNow()) {
        const double a = s->clock();
        return a >= 0 ? a : 0;
    }
    return s->position.load();
}

double Player::duration() const
{
    if (vlcActive_)
        return vlc_->duration();
    auto s = session_;
    return s ? s->durationSec.load() : 0;
}

bool Player::isAudioOnly() const
{
    if (vlcActive_)
        return vlc_->isAudioOnly();
    auto s = session_;
    if (!s)
        return false;
    std::lock_guard<std::mutex> l(s->infoMu);
    return s->audioOnly;
}

QString Player::title() const
{
    if (vlcActive_)
        return {};
    auto s = session_;
    if (!s)
        return {};
    std::lock_guard<std::mutex> l(s->infoMu);
    return s->title;
}

QImage Player::coverArt() const
{
    if (vlcActive_)
        return {};
    auto s = session_;
    if (!s)
        return {};
    std::lock_guard<std::mutex> l(s->infoMu);
    return s->cover;
}

PlayerStats Player::stats() const
{
    if (vlcActive_)
        return vlc_->stats();
    PlayerStats out;
    if (auto s = session_) {
        std::lock_guard<std::mutex> l(s->infoMu);
        out = s->stats;
        out.framesDecoded = s->framesDecoded.load();
        out.framesShown = s->framesShown.load();
        out.framesDropped = s->framesDropped.load();
        if (s->reader)
            out.badBlocks = s->reader->badBlocks();
    }
    out.audioFrames = audio_->framesWritten();
    out.underruns = audio_->underruns();
    return out;
}

QVector<SubtitleCue> Player::activeCues() const
{
    if (vlcActive_)
        return {}; // VLC draws them into the picture
    QVector<SubtitleCue> out;
    auto s = session_;
    if (!s)
        return out;
    const double now = position();
    const bool forcedOnly = s->forcedOnly.load();
    std::lock_guard<std::mutex> l(s->cueMu);
    for (const SubtitleCue &c : s->cues)
        if (c.start <= now && c.end > now && (!forcedOnly || c.forced))
            out << c;
    return out;
}

} // namespace wmp
