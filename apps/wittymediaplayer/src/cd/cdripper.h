// What an audio CD holds, how its files get named, and the job that copies tracks to
// them. The helper program reads the disc and sends the samples here to be encoded, so
// the file only ever appears where the viewer chose. A file is written under a temporary
// name and never replaces anything.
#pragma once

#include "audioencoder.h"

#include <QJsonObject>
#include <QMetaType>
#include <QObject>
#include <QString>
#include <QStringList>
#include <QVector>
#include <atomic>

class QThread;

namespace wmp {

struct CdTrack {
    int number = 0;
    int start = 0, end = 0; // in sectors, 75 to the second
    QString title, performer, isrc;
    bool preEmphasis = false;
    double seconds() const { return (end - start) / 75.0; }
};

struct CdInfo {
    QString path;
    bool drive = false;
    QString title, performer, date;
    QString discId, toc;
    QVector<CdTrack> tracks; // the ones with sound
    QStringList warnings;
    double startOf(int index) const { return tracks.isEmpty() ? 0 : (tracks[index].start - tracks[0].start) / 75.0; }
    static CdInfo fromJson(const QJsonObject &o);
};

struct RipNames {
    QString artist, albumArtist, album, title, year;
    int track = 0, tracks = 0, disc = 0;
};

// The file's place under the chosen folder, from a template such as
// {albumartist}/{album}/{track} {title}. Names come from outside, so each part is made
// safe here: it can't climb out of the folder, or be a name Windows won't take.
QString ripRelativePath(const QString &nameTemplate, const RipNames &names, const QString &extension);
QString defaultRipTemplate();
// path, or path with a number added if something's already there.
QString uniquePath(const QString &path);

struct RipTrackJob {
    int number = 0;
    RipNames names;
    TrackTags tags;
};

struct RipJob {
    QString source;
    QVector<RipTrackJob> tracks;
    QString format, quality, folder, nameTemplate;
    bool secure = true;
    int offset = 0;
    QString album; // for the report
};

struct RipResult {
    int number = 0;
    bool ok = false;
    QString path, crc, quality, error;
    bool verified = false;
    int retries = 0, unreadable = 0;
};

class CdRipper : public QObject {
    Q_OBJECT
public:
    explicit CdRipper(QObject *parent = nullptr);
    ~CdRipper() override;

    void start(const RipJob &job);
    void stop(); // the partial file goes too
    bool running() const { return thread_ != nullptr; }

signals:
    void trackStarted(int number);
    void trackProgress(int number, int percent);
    void trackFinished(const wmp::RipResult &result);
    // Everything's over. An empty message is a job finished without trouble.
    void finished(bool stopped, const QString &message, const QString &reportPath);

private:
    void run(const RipJob &job);

    QThread *thread_ = nullptr;
    std::atomic<bool> stop_{false};
};

} // namespace wmp

Q_DECLARE_METATYPE(wmp::RipResult)
