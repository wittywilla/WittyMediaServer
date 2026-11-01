#include "audioencoder.h"

#include "media/ffmpeg.h"

extern "C" {
#include <libavutil/audio_fifo.h>
#include <libavutil/opt.h>
}

#include <QBuffer>
#include <QImage>
#include <QtEndian>

namespace wmp {

namespace {

constexpr int kInRate = 44100;
constexpr int kBytesPerFrame = 4;
constexpr int kPcmChunk = 1152; // an encoder that takes any size still gets steady pieces

const char *const kNoQuality = "";

QVector<RipFormat> buildFormats()
{
    QVector<RipFormat> f;
    f.push_back({QStringLiteral("flac"), QStringLiteral("FLAC (lossless)"), QStringLiteral("flac"), QStringLiteral("flac"), QStringLiteral("flac"), true,
                 {{QStringLiteral("5"), QStringLiteral("Balanced"), 0, -1, 5},
                  {QStringLiteral("8"), QStringLiteral("Smallest files, slower"), 0, -1, 8},
                  {QStringLiteral("0"), QStringLiteral("Fastest, bigger files"), 0, -1, 0}},
                 0});
    f.push_back({QStringLiteral("alac"), QStringLiteral("ALAC (lossless, in .m4a)"), QStringLiteral("m4a"), QStringLiteral("ipod"), QStringLiteral("alac"), true,
                 {{QString::fromLatin1(kNoQuality), QStringLiteral("Lossless")}},
                 0});
    f.push_back({QStringLiteral("wav"), QStringLiteral("WAV (uncompressed)"), QStringLiteral("wav"), QStringLiteral("wav"), QStringLiteral("pcm_s16le"), true,
                 {{QString::fromLatin1(kNoQuality), QStringLiteral("Exactly as on the disc")}},
                 0});
    f.push_back({QStringLiteral("mp3"), QStringLiteral("MP3"), QStringLiteral("mp3"), QStringLiteral("mp3"), QStringLiteral("libmp3lame"), false,
                 {{QStringLiteral("320"), QStringLiteral("320 kbit/s"), 320000},
                  {QStringLiteral("256"), QStringLiteral("256 kbit/s"), 256000},
                  {QStringLiteral("192"), QStringLiteral("192 kbit/s"), 192000},
                  {QStringLiteral("128"), QStringLiteral("128 kbit/s"), 128000},
                  {QStringLiteral("v0"), QStringLiteral("Variable, about 245 kbit/s"), 0, 0},
                  {QStringLiteral("v2"), QStringLiteral("Variable, about 190 kbit/s"), 0, 2}},
                 0});
    f.push_back({QStringLiteral("aac"), QStringLiteral("AAC (in .m4a)"), QStringLiteral("m4a"), QStringLiteral("ipod"), QStringLiteral("aac"), false,
                 {{QStringLiteral("256"), QStringLiteral("256 kbit/s"), 256000},
                  {QStringLiteral("192"), QStringLiteral("192 kbit/s"), 192000},
                  {QStringLiteral("128"), QStringLiteral("128 kbit/s"), 128000}},
                 0});
    f.push_back({QStringLiteral("opus"), QStringLiteral("Opus"), QStringLiteral("opus"), QStringLiteral("ogg"), QStringLiteral("libopus"), false,
                 {{QStringLiteral("192"), QStringLiteral("192 kbit/s"), 192000},
                  {QStringLiteral("128"), QStringLiteral("128 kbit/s"), 128000},
                  {QStringLiteral("96"), QStringLiteral("96 kbit/s"), 96000}},
                 1});
    f.push_back({QStringLiteral("vorbis"), QStringLiteral("Ogg Vorbis"), QStringLiteral("ogg"), QStringLiteral("ogg"), QStringLiteral("libvorbis"), false,
                 {{QStringLiteral("q8"), QStringLiteral("Quality 8, about 256 kbit/s"), 0, 8},
                  {QStringLiteral("q6"), QStringLiteral("Quality 6, about 192 kbit/s"), 0, 6},
                  {QStringLiteral("q4"), QStringLiteral("Quality 4, about 128 kbit/s"), 0, 4}},
                 1});
    return f;
}

QString ffError(int code)
{
    char buf[AV_ERROR_MAX_STRING_SIZE] = {};
    av_strerror(code, buf, sizeof buf);
    return QString::fromUtf8(buf);
}

void setTag(AVDictionary **d, const char *key, const QString &value)
{
    if (!value.isEmpty())
        av_dict_set(d, key, value.toUtf8().constData(), 0);
}

// The picture block Ogg files carry their cover in, as a tag.
QByteArray oggPictureBlock(const QByteArray &cover, const QImage &img, bool png)
{
    QByteArray b;
    const auto put32 = [&b](quint32 v) {
        char c[4];
        qToBigEndian(v, c);
        b.append(c, 4);
    };
    const QByteArray mime = png ? QByteArrayLiteral("image/png") : QByteArrayLiteral("image/jpeg");
    put32(3); // the front cover
    put32(quint32(mime.size()));
    b.append(mime);
    put32(0); // no description
    put32(quint32(img.width()));
    put32(quint32(img.height()));
    put32(24);
    put32(0);
    put32(quint32(cover.size()));
    b.append(cover);
    return b.toBase64();
}

} // namespace

const QVector<RipFormat> &ripFormats()
{
    static const QVector<RipFormat> formats = buildFormats();
    return formats;
}

const RipFormat *ripFormat(const QString &id)
{
    for (const RipFormat &f : ripFormats())
        if (f.id == id)
            return &f;
    return nullptr;
}

AudioEncoder::AudioEncoder() = default;

AudioEncoder::~AudioEncoder()
{
    close();
}

void AudioEncoder::close()
{
    if (fmt_) {
        if (fmt_->pb)
            avio_closep(&fmt_->pb);
        avformat_free_context(fmt_);
        fmt_ = nullptr;
    }
    avcodec_free_context(&ctx_);
    swr_free(&swr_);
    if (fifo_) {
        av_audio_fifo_free(fifo_);
        fifo_ = nullptr;
    }
    av_frame_free(&frame_);
    av_packet_free(&pkt_);
    stream_ = coverStream_ = nullptr;
    headerWritten_ = false;
    pts_ = 0; // one encoder rips a whole disc, and each track starts at zero
}

bool AudioEncoder::fail(const QString &message, int code)
{
    error_ = code ? QStringLiteral("%1 (%2)").arg(message, ffError(code)) : message;
    return false;
}

bool AudioEncoder::addCover(const QByteArray &cover, bool oggStyle)
{
    const QImage img = QImage::fromData(cover);
    if (img.isNull())
        return true; // a bad picture is no reason to lose the music
    const bool png = cover.startsWith("\x89PNG");
    if (oggStyle) {
        const QByteArray block = oggPictureBlock(cover, img, png);
        av_dict_set(&stream_->metadata, "METADATA_BLOCK_PICTURE", block.constData(), 0);
        return true;
    }
    if (!png && !cover.startsWith("\xff\xd8"))
        return true;
    AVStream *st = avformat_new_stream(fmt_, nullptr);
    if (!st)
        return fail(QStringLiteral("Couldn't add the cover."));
    st->disposition = AV_DISPOSITION_ATTACHED_PIC;
    st->time_base = {1, 90000};
    st->codecpar->codec_type = AVMEDIA_TYPE_VIDEO;
    st->codecpar->codec_id = png ? AV_CODEC_ID_PNG : AV_CODEC_ID_MJPEG;
    st->codecpar->width = img.width();
    st->codecpar->height = img.height();
    coverStream_ = st;
    coverData_ = cover;
    return true;
}

bool AudioEncoder::open(const QString &path, const QString &formatId, const QString &qualityId, const TrackTags &tags, QString *why)
{
    close();
    error_.clear();
    const auto done = [&](bool ok) {
        if (!ok) {
            if (why)
                *why = error_;
            close();
        }
        return ok;
    };
    const RipFormat *f = ripFormat(formatId);
    if (!f)
        return done(fail(QStringLiteral("This player doesn't write that format.")));
    const AVCodec *codec = avcodec_find_encoder_by_name(f->codec.toLatin1().constData());
    if (!codec)
        return done(fail(QStringLiteral("This copy of the player has no %1 encoder.").arg(f->name)));
    ctx_ = avcodec_alloc_context3(codec);
    pkt_ = av_packet_alloc();
    frame_ = av_frame_alloc();

    // What the encoder takes, nearest to what the disc has.
    const AVSampleFormat *fmts = nullptr;
    int count = 0;
    avcodec_get_supported_config(nullptr, codec, AV_CODEC_CONFIG_SAMPLE_FORMAT, 0, reinterpret_cast<const void **>(&fmts), &count);
    AVSampleFormat chosen = AV_SAMPLE_FMT_NONE;
    for (AVSampleFormat want : {AV_SAMPLE_FMT_S16, AV_SAMPLE_FMT_S16P, AV_SAMPLE_FMT_FLTP, AV_SAMPLE_FMT_FLT, AV_SAMPLE_FMT_S32P, AV_SAMPLE_FMT_S32}) {
        for (int i = 0; i < count && chosen == AV_SAMPLE_FMT_NONE; i++)
            if (fmts[i] == want)
                chosen = want;
        if (chosen != AV_SAMPLE_FMT_NONE)
            break;
    }
    if (chosen == AV_SAMPLE_FMT_NONE)
        return done(fail(QStringLiteral("The %1 encoder can't take this audio.").arg(f->name)));
    const int *rates = nullptr;
    int rateCount = 0;
    avcodec_get_supported_config(nullptr, codec, AV_CODEC_CONFIG_SAMPLE_RATE, 0, reinterpret_cast<const void **>(&rates), &rateCount);
    int rate = rateCount > 0 ? 0 : kInRate;
    for (int i = 0; i < rateCount; i++) {
        if (rates[i] == kInRate) {
            rate = kInRate;
            break;
        }
        rate = qMax(rate, rates[i]);
    }

    ctx_->sample_fmt = chosen;
    ctx_->sample_rate = rate;
    ctx_->time_base = {1, rate};
    av_channel_layout_default(&ctx_->ch_layout, 2);
    RipQuality quality = f->qualities.value(f->defaultQuality);
    for (const RipQuality &q : f->qualities)
        if (q.id == qualityId)
            quality = q;
    if (quality.bitrate > 0)
        ctx_->bit_rate = quality.bitrate;
    if (quality.compression >= 0)
        ctx_->compression_level = quality.compression;
    if (quality.vbr >= 0) {
        ctx_->flags |= AV_CODEC_FLAG_QSCALE;
        ctx_->global_quality = quality.vbr * FF_QP2LAMBDA;
    }

    const QByteArray utf8 = path.toUtf8();
    int rc = avformat_alloc_output_context2(&fmt_, nullptr, f->muxer.toLatin1().constData(), utf8.constData());
    if (rc < 0 || !fmt_)
        return done(fail(QStringLiteral("Couldn't set up the file."), rc));
    if (fmt_->oformat->flags & AVFMT_GLOBALHEADER)
        ctx_->flags |= AV_CODEC_FLAG_GLOBAL_HEADER;
    rc = avcodec_open2(ctx_, codec, nullptr);
    if (rc < 0)
        return done(fail(QStringLiteral("The %1 encoder wouldn't start.").arg(f->name), rc));
    stream_ = avformat_new_stream(fmt_, nullptr);
    if (!stream_)
        return done(fail(QStringLiteral("Couldn't set up the file.")));
    stream_->time_base = ctx_->time_base;
    avcodec_parameters_from_context(stream_->codecpar, ctx_);

    AVDictionary **dict = &fmt_->metadata;
    setTag(dict, "title", tags.title);
    setTag(dict, "artist", tags.artist);
    setTag(dict, "album", tags.album);
    setTag(dict, "album_artist", tags.albumArtist);
    setTag(dict, "date", tags.date);
    setTag(dict, "ISRC", tags.isrc);
    if (tags.track > 0)
        setTag(dict, "track", tags.trackTotal > 0 ? QStringLiteral("%1/%2").arg(tags.track).arg(tags.trackTotal) : QString::number(tags.track));
    if (tags.disc > 0)
        setTag(dict, "disc", tags.discTotal > 0 ? QStringLiteral("%1/%2").arg(tags.disc).arg(tags.discTotal) : QString::number(tags.disc));
    if (!tags.cover.isEmpty() && f->id != QLatin1String("wav") && !addCover(tags.cover, f->muxer == QLatin1String("ogg")))
        return done(false);

    swr_alloc_set_opts2(&swr_, &ctx_->ch_layout, ctx_->sample_fmt, ctx_->sample_rate, &ctx_->ch_layout, AV_SAMPLE_FMT_S16, kInRate, 0, nullptr);
    if (!swr_ || swr_init(swr_) < 0)
        return done(fail(QStringLiteral("Couldn't convert the audio for this format.")));
    frameSize_ = ctx_->frame_size > 0 ? ctx_->frame_size : kPcmChunk;
    fifo_ = av_audio_fifo_alloc(ctx_->sample_fmt, 2, frameSize_ * 4);
    if (!fifo_)
        return done(fail(QStringLiteral("Not enough memory.")));

    rc = avio_open(&fmt_->pb, utf8.constData(), AVIO_FLAG_WRITE);
    if (rc < 0)
        return done(fail(QStringLiteral("Couldn't write this file."), rc));
    rc = avformat_write_header(fmt_, nullptr);
    if (rc < 0)
        return done(fail(QStringLiteral("Couldn't start writing this file."), rc));
    headerWritten_ = true;
    if (coverStream_) {
        AVPacket *pic = av_packet_alloc();
        av_new_packet(pic, int(coverData_.size()));
        memcpy(pic->data, coverData_.constData(), size_t(coverData_.size()));
        pic->stream_index = coverStream_->index;
        pic->flags |= AV_PKT_FLAG_KEY;
        rc = av_interleaved_write_frame(fmt_, pic);
        av_packet_free(&pic);
        if (rc < 0)
            return done(fail(QStringLiteral("Couldn't write the cover."), rc));
    }
    return true;
}

bool AudioEncoder::write(const char *pcm, qint64 bytes)
{
    if (!ctx_)
        return fail(QStringLiteral("Nothing is open."));
    const int inFrames = int(bytes / kBytesPerFrame);
    if (inFrames == 0)
        return true;
    const auto *in = reinterpret_cast<const uint8_t *>(pcm);
    const int room = swr_get_out_samples(swr_, inFrames);
    uint8_t **out = nullptr;
    int rc = av_samples_alloc_array_and_samples(&out, nullptr, 2, room, ctx_->sample_fmt, 0);
    if (rc < 0)
        return fail(QStringLiteral("Not enough memory."));
    const int got = swr_convert(swr_, out, room, &in, inFrames);
    bool ok = got >= 0;
    if (ok && got > 0)
        ok = av_audio_fifo_write(fifo_, reinterpret_cast<void **>(out), got) == got;
    av_freep(&out[0]);
    av_freep(&out);
    if (!ok)
        return fail(QStringLiteral("The audio couldn't be converted."));
    return drain(false);
}

bool AudioEncoder::encodeFrame(AVFrame *frame)
{
    int rc = avcodec_send_frame(ctx_, frame);
    if (rc < 0)
        return fail(QStringLiteral("The encoder stopped."), rc);
    while (true) {
        rc = avcodec_receive_packet(ctx_, pkt_);
        if (rc == AVERROR(EAGAIN) || rc == AVERROR_EOF)
            return true;
        if (rc < 0)
            return fail(QStringLiteral("The encoder stopped."), rc);
        av_packet_rescale_ts(pkt_, ctx_->time_base, stream_->time_base);
        pkt_->stream_index = stream_->index;
        rc = av_interleaved_write_frame(fmt_, pkt_);
        if (rc < 0)
            return fail(QStringLiteral("Couldn't write the file. Is the disk full?"), rc);
    }
}

bool AudioEncoder::drain(bool flush)
{
    while (av_audio_fifo_size(fifo_) >= (flush ? 1 : frameSize_)) {
        const int n = qMin(frameSize_, av_audio_fifo_size(fifo_));
        av_frame_unref(frame_);
        frame_->nb_samples = n;
        frame_->format = ctx_->sample_fmt;
        frame_->sample_rate = ctx_->sample_rate;
        av_channel_layout_copy(&frame_->ch_layout, &ctx_->ch_layout);
        if (av_frame_get_buffer(frame_, 0) < 0)
            return fail(QStringLiteral("Not enough memory."));
        if (av_audio_fifo_read(fifo_, reinterpret_cast<void **>(frame_->data), n) != n)
            return fail(QStringLiteral("The audio couldn't be converted."));
        frame_->pts = pts_;
        pts_ += n;
        if (!encodeFrame(frame_))
            return false;
    }
    return true;
}

bool AudioEncoder::finish()
{
    if (!ctx_ || !headerWritten_)
        return fail(QStringLiteral("Nothing is open."));
    // What the converter still holds.
    const int room = swr_get_out_samples(swr_, 0);
    if (room > 0) {
        uint8_t **out = nullptr;
        if (av_samples_alloc_array_and_samples(&out, nullptr, 2, room, ctx_->sample_fmt, 0) >= 0) {
            const int got = swr_convert(swr_, out, room, nullptr, 0);
            if (got > 0)
                av_audio_fifo_write(fifo_, reinterpret_cast<void **>(out), got);
            av_freep(&out[0]);
            av_freep(&out);
        }
    }
    if (!drain(true) || !encodeFrame(nullptr))
        return false;
    const int rc = av_write_trailer(fmt_);
    if (rc < 0)
        return fail(QStringLiteral("Couldn't finish writing the file."), rc);
    const int closed = avio_closep(&fmt_->pb);
    close();
    if (closed < 0)
        return fail(QStringLiteral("Couldn't finish writing the file."), closed);
    return true;
}

} // namespace wmp
