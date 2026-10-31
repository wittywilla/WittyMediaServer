// Optional playback through the VLC libraries (libVLC, by VideoLAN and its
// contributors), loaded from a VLC the viewer has installed. None of it ships with
// this player, the same arrangement as Java for BD-J menus.
//
// All input reaches VLC through this player's own reader, so VLC never opens a file, a
// web address or a disc itself. Its disc modules, which can use libdvdcss, are never
// chosen, and a protected disc has already been refused before anything gets here.
#pragma once

#include "mediaspec.h"
#include "player.h"

#include <QLibrary>
#include <QObject>
#include <QString>
#include <QTimer>
#include <atomic>
#include <memory>

namespace wmp {

struct VlcSession;
struct VlcApi;

class VlcPlayback : public QObject {
    Q_OBJECT
public:
    explicit VlcPlayback(Player *owner);
    ~VlcPlayback() override;

    // Where VLC is installed on this computer, or empty.
    static QString detectFolder();

    // Loads the libraries from a folder. Says why not, in plain words, on failure.
    bool load(const QString &folder, QString *why);
    bool loaded() const;
    QString version() const;

    void open(const MediaSpec &spec, VideoSink *sink);
    void stop();
    void setPaused(bool paused);
    void seek(double sec);
    void seekDisc(int64_t offset, double target);
    void restart();
    void setVolume(double v);
    void setAudioTrack(int id);
    void setSubtitleTrack(int id);
    void setDiscSubtitle(int streamId, bool forcedOnly);

    bool hasMedia() const;
    double position() const;
    double duration() const;
    bool isAudioOnly() const;
    QVector<TrackInfo> audioTracks() const;
    QVector<TrackInfo> subtitleTracks() const;
    int currentAudioTrack() const;
    int currentSubtitleTrack() const;
    PlayerStats stats() const;

    // The stream ids VLC gives a DVD's streams, from the ids FFmpeg and the engine use.
    // Public for tests.
    static int discStreamId(const QString &container, int id);
    // A byte place in a stream of segments as a time on its timeline. Public for tests.
    static double timeAt(const QVector<Segment> &segments, double fraction);

private:
    void poll();
    void applyPending();

    Player *owner_;
    QLibrary core_, lib_;
    std::unique_ptr<VlcApi> api_;
    void *instance_ = nullptr;
    QString version_;
    std::shared_ptr<VlcSession> session_;
    std::atomic<int> retiring_{0};
    QTimer poll_;
    double volume_ = 1.0;
    bool paused_ = false;
};

} // namespace wmp
