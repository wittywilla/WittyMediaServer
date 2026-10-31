// Plays one thing at a time: demuxes on its own thread, decodes picture and sound on
// theirs, and keeps them together. Works without Qt's event loop in the decode path;
// everything reaches the window through queued signals or the VideoSink.
#pragma once

#include "ffmpeg.h"
#include "mediaspec.h"

#include <QHash>
#include <QImage>
#include <QObject>
#include <QRectF>
#include <QString>
#include <QTimer>
#include <QVector>
#include <atomic>
#include <memory>

namespace wmp {

class AudioOutput;
class VlcPlayback;
struct Session;

// Where finished pictures go. presentFrame is called on the video thread.
class VideoSink {
public:
    virtual ~VideoSink() = default;
    virtual void presentFrame(const FramePtr &frame) = 0;
};

struct TrackInfo {
    int index = -1; // stream index in a file, or sidecar number with external set
    QString language;
    QString title;
    QString codec;
    int channels = 0;
    bool external = false;
    bool forced = false;
    bool isDefault = false;
};

// One subtitle on screen: a picture or words.
struct SubtitleCue {
    double start = 0;
    double end = 1e12;
    QImage image;
    QRectF rect; // in frame units: 0..1 of the picture
    QString text;
    bool forced = false;
    uint64_t id = 0;
};

struct PlayerStats {
    int64_t framesDecoded = 0;
    int64_t framesShown = 0;
    int64_t framesDropped = 0;
    int64_t audioFrames = 0;
    int64_t underruns = 0;
    int64_t badBlocks = 0;
    int width = 0, height = 0;
    bool hwDecode = false;
    bool deinterlacing = false;
    QString videoCodec, audioCodec;
};

class Player : public QObject {
    Q_OBJECT
public:
    explicit Player(QObject *parent = nullptr);
    ~Player() override;

    void setVideoSink(VideoSink *sink) { sink_ = sink; }
    AudioOutput *audio() const { return audio_; }

    // noDevice runs sound on a timer rather than real hardware.
    void initAudio(bool noDevice);

    void open(const MediaSpec &spec);
    void stop();
    void setPaused(bool paused);
    bool paused() const { return paused_; }
    bool hasMedia() const;

    // An ordinary file seeks by time. A disc seeks to a place the engine found.
    void seek(double sec);
    void seekDisc(int64_t offset, double at, int64_t pts, double target);
    // From the start again: a looping menu.
    void restart();

    void setVolume(double v);

    // The VLC libraries, when the viewer chooses them (see vlcplayback.h). enableVlc only
    // loads them; useVlc says whether the next thing opened plays through them.
    bool enableVlc(const QString &folder, QString *why);
    void useVlc(bool on) { useVlc_ = on; }
    bool playingThroughVlc() const { return vlcActive_; }
    QString vlcVersion() const;

    // Ordinary files: stream indices from the track lists; -1 = off.
    void setAudioTrack(int index);
    void setSubtitleTrack(int index, bool external);
    QVector<TrackInfo> audioTracks() const;
    QVector<TrackInfo> subtitleTracks() const;
    int currentAudioTrack() const;
    int currentSubtitleTrack() const;
    bool currentSubtitleExternal() const;

    // Discs: the subtitle stream to decode (id or PID, 0 = none). With forcedOnly,
    // only the disc's forced captions show.
    void setDiscSubtitle(int id, bool forcedOnly);

    double position() const;
    double duration() const;
    bool isAudioOnly() const;
    QString title() const;
    QImage coverArt() const;
    PlayerStats stats() const;
    // Cues to show at this moment.
    QVector<SubtitleCue> activeCues() const;

signals:
    void opened();
    void tracksChanged();
    void ended();
    void failed(const QString &message);
    // Part of a disc wouldn't read; playback skipped it.
    void discTrouble(const QString &message);
    void discLost();
    void firstFrame();

private:
    friend struct Session;
    void retire(std::shared_ptr<Session> s);

    std::shared_ptr<Session> session_;
    VlcPlayback *vlc_ = nullptr;
    bool useVlc_ = false;
    bool vlcActive_ = false;
    AudioOutput *audio_ = nullptr;
    VideoSink *sink_ = nullptr;
    bool paused_ = false;
    double volume_ = 1.0;
    uint64_t generation_ = 0;
    QTimer watch_;
    std::atomic<int> retiring_{0};
    int64_t reportedBad_ = 0;
    bool reportedLost_ = false;
};

// Language names for menus.
QString languageName(const QString &code);

} // namespace wmp

Q_DECLARE_METATYPE(wmp::FramePtr)
