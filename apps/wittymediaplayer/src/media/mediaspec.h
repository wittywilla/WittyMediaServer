// What to play: an ordinary file, or a stretch of a disc described by the engine.
#pragma once

#include <QJsonObject>
#include <QString>
#include <QStringList>
#include <QVector>
#include <cstdint>

namespace wmp {

struct Extent {
    int file = 0;
    int64_t offset = 0;
    int64_t length = 0;
};

struct Segment {
    int64_t bytes = 0;
    double startSec = 0;
    double durationSec = 0;
    int64_t basePts = -1; // on a 90 kHz clock; -1 until the stream shows it
};

struct MediaSpec {
    // An ordinary file, when set. Everything else then comes from the file.
    QString filePath;
    // That path is an audio CD, a drive or an image, read through the helper as one WAV.
    bool cd = false;

    // A disc: byte ranges to decode as one stream.
    QString token;
    QString container; // mpeg | mpegts
    QStringList files;
    QVector<Extent> extents;
    QVector<Segment> segments;
    int videoId = 0;
    int audioId = 0;
    bool audioFirst = false;
    bool frontOnly = false;
    bool loop = false;
    double durationSec = 0;
    double stillSec = 0; // hold after the clip; -1 = until the viewer acts
    bool deinterlace = true;
    QString aspect; // 4:3 | 16:9 | empty = from the picture
    double startSec = 0;
    int64_t startOffset = 0;
    double startAt = 0;
    int64_t startPts = -1;
    // The disc's colour table for its sub-pictures, as RRGGBB.
    QStringList subPalette;

    bool isDisc() const { return filePath.isEmpty(); }
    bool valid() const { return !filePath.isEmpty() || !extents.isEmpty(); }

    static MediaSpec fromJson(const QJsonObject &media);
};

} // namespace wmp
