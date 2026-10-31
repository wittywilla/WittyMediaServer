// Turns CD audio into a music file: the formats and qualities on offer, and one encoder
// writing one file, tags and cover included. Nothing here knows about discs.
#pragma once

#include <QByteArray>
#include <QString>
#include <QVector>

struct AVAudioFifo;
struct AVCodecContext;
struct AVFormatContext;
struct AVFrame;
struct AVPacket;
struct AVStream;
struct SwrContext;

namespace wmp {

struct RipQuality {
    QString id;
    QString label;
    int bitrate = 0; // bits per second, for a set rate
    int vbr = -1;    // the encoder's own quality scale, when it isn't a set rate
    int compression = -1;
};

struct RipFormat {
    QString id;
    QString name;
    QString extension;
    QString muxer;
    QString codec;
    bool lossless = false;
    QVector<RipQuality> qualities;
    int defaultQuality = 0;
};

const QVector<RipFormat> &ripFormats();
const RipFormat *ripFormat(const QString &id);

struct TrackTags {
    QString title, artist, album, albumArtist, date, isrc;
    int track = 0, trackTotal = 0, disc = 0, discTotal = 0;
    QByteArray cover; // a JPEG or PNG
};

class AudioEncoder {
public:
    AudioEncoder();
    ~AudioEncoder();
    AudioEncoder(const AudioEncoder &) = delete;
    AudioEncoder &operator=(const AudioEncoder &) = delete;

    // Writes to path, which should not exist yet.
    bool open(const QString &path, const QString &formatId, const QString &qualityId, const TrackTags &tags, QString *why);
    // 16-bit stereo at 44.1 kHz, any amount at a time.
    bool write(const char *pcm, qint64 bytes);
    // Ends the file properly. It isn't complete until this returns true.
    bool finish();
    // Gives up, closing what's open. The caller removes the partial file.
    void close();
    QString error() const { return error_; }

private:
    bool fail(const QString &message, int code = 0);
    bool addCover(const QByteArray &cover, bool oggStyle);
    bool drain(bool flush);
    bool encodeFrame(AVFrame *frame);

    AVFormatContext *fmt_ = nullptr;
    AVCodecContext *ctx_ = nullptr;
    AVStream *stream_ = nullptr;
    AVStream *coverStream_ = nullptr;
    SwrContext *swr_ = nullptr;
    AVAudioFifo *fifo_ = nullptr;
    AVFrame *frame_ = nullptr;
    AVPacket *pkt_ = nullptr;
    QByteArray coverData_;
    int frameSize_ = 0;
    int64_t pts_ = 0;
    bool headerWritten_ = false;
    QString error_;
};

} // namespace wmp
