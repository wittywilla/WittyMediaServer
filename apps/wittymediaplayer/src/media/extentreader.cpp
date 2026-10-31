#include "extentreader.h"

#include "ffmpeg.h"

#include <QFileInfo>
#include <algorithm>
#include <cstring>

#ifdef _WIN32
#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#endif

namespace wmp {

namespace {
// A damaged block is skipped by this much: a DVD error-correction block is 32 KiB,
// and a Blu-ray's 64 KiB is two of them. Skipping less only means slower recovery.
constexpr int kSkipBytes = 32 * 1024;
// This many skips in a row with the file gone means the disc has been taken out.
constexpr int kGiveUpAfter = 4;
constexpr int kAvioBuffer = 128 * 1024;

int avioRead(void *opaque, uint8_t *buf, int size)
{
    auto *r = static_cast<ByteSource *>(opaque);
    int n = r->read(buf, size);
    if (n < 0)
        return AVERROR_EXIT;
    return n == 0 ? AVERROR_EOF : n;
}

int64_t avioSeek(void *opaque, int64_t pos, int whence)
{
    auto *r = static_cast<ByteSource *>(opaque);
    if (whence & AVSEEK_SIZE)
        return r->size();
    switch (whence & ~AVSEEK_FORCE) {
    case SEEK_SET:
        return r->seek(pos);
    case SEEK_END:
        return r->seek(r->size() + pos);
    default:
        return AVERROR(EINVAL);
    }
}
} // namespace

ExtentReader::ExtentReader(const QStringList &files, const QVector<Extent> &extents)
    : names_(files), extents_(extents)
{
    starts_.reserve(extents.size());
    for (const Extent &e : extents_) {
        starts_.push_back(total_);
        total_ += e.length;
    }
    files_.resize(names_.size());
}

ByteSource::~ByteSource()
{
    if (avio_) {
        av_freep(&avio_->buffer);
        avio_context_free(&avio_);
    }
}

AVIOContext *ByteSource::avio()
{
    if (!avio_) {
        auto *buffer = static_cast<unsigned char *>(av_malloc(kAvioBuffer));
        avio_ = avio_alloc_context(buffer, kAvioBuffer, 0, this, avioRead, nullptr, avioSeek);
    }
    return avio_;
}

bool ExtentReader::locate(int64_t pos, int *extent, int64_t *within) const
{
    if (pos < 0 || pos >= total_ || starts_.empty())
        return false;
    auto it = std::upper_bound(starts_.begin(), starts_.end(), pos);
    int k = int(it - starts_.begin()) - 1;
    *extent = k;
    *within = pos - starts_[size_t(k)];
    return true;
}

int64_t ExtentReader::seek(int64_t pos)
{
    if (pos < 0 || pos > total_)
        return AVERROR(EINVAL);
    pos_ = pos;
    return pos_;
}

bool ExtentReader::fileGone(int index) const
{
    return !QFileInfo::exists(names_.at(index));
}

int ExtentReader::readOnce(uint8_t *buf, int want)
{
    int k = 0;
    int64_t within = 0;
    if (!locate(pos_, &k, &within))
        return 0;
    const Extent &e = extents_.at(k);
    if (e.file < 0 || e.file >= int(files_.size()))
        return -2;
    auto &f = files_[size_t(e.file)];
    if (!f) {
        f = std::make_unique<QFile>(names_.at(e.file));
        if (!f->open(QIODevice::ReadOnly | QIODevice::Unbuffered)) {
            f.reset();
            return -2;
        }
    }
    int take = int(std::min<int64_t>(want, e.length - within));
    if (!f->seek(e.offset + within))
        return -2;
    qint64 n = f->read(reinterpret_cast<char *>(buf), take);
    if (n <= 0)
        return -2;
    return int(n);
}

int ExtentReader::read(uint8_t *buf, int want)
{
    if (stopped_.load())
        return -1;
#ifdef _WIN32
    if (!readingThread_.load())
        readingThread_.store(OpenThread(THREAD_TERMINATE, FALSE, GetCurrentThreadId()));
#endif
    for (int tries = 0;; tries++) {
        if (stopped_.load())
            return -1;
        int n = readOnce(buf, want);
        if (n >= 0) {
            if (n > 0) {
                pos_ += n;
                failedInARow_ = 0;
            }
            return n;
        }
        if (tries < 1)
            continue;
        // It won't read. Hand the demuxer zeros up to the next block and go on: both
        // kinds of stream resynchronise by themselves.
        int k = 0;
        int64_t within = 0;
        if (locate(pos_, &k, &within) && fileGone(extents_.at(k).file) && ++failedInARow_ >= kGiveUpAfter) {
            lost_.store(true);
            return -1;
        }
        int64_t next = (pos_ / kSkipBytes + 1) * kSkipBytes;
        int fill = int(std::min<int64_t>({next - pos_, int64_t(want), total_ - pos_}));
        if (fill <= 0)
            return 0;
        std::memset(buf, 0, size_t(fill));
        pos_ += fill;
        bad_.fetch_add(1);
        return fill;
    }
}

void ExtentReader::stop()
{
    stopped_.store(true);
#ifdef _WIN32
    // A drive retrying a bad sector can hold a read for a long time.
    if (void *t = readingThread_.exchange(nullptr)) {
        CancelSynchronousIo(t);
        CloseHandle(t);
    }
#endif
}

} // namespace wmp
