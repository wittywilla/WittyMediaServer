// A disc's byte ranges as one file for the demuxer. It copes with what real drives
// do: a sector that won't read is skipped and counted instead of ending playback,
// and a disc pulled from the drive is reported as gone.
#pragma once

#include "mediaspec.h"

#include <QFile>
#include <atomic>
#include <cstdint>
#include <memory>
#include <vector>

struct AVIOContext;

namespace wmp {

// Something the demuxer reads bytes from.
class ByteSource {
public:
    virtual ~ByteSource();

    virtual int64_t size() const = 0;
    virtual int read(uint8_t *buf, int want) = 0; // bytes read, 0 at the end, -1 when stopped
    virtual int64_t seek(int64_t pos) = 0;
    // Stop any read in progress and refuse further ones. Safe from another thread.
    virtual void stop() = 0;
    // Skipped because they wouldn't read.
    virtual int64_t badBlocks() const { return 0; }
    virtual bool discLost() const { return false; }
    // Called on the reading thread once it won't read any more.
    virtual void finishedReading() {}

    // A context for FFmpeg, owned by this source.
    AVIOContext *avio();

private:
    AVIOContext *avio_ = nullptr;
};

class ExtentReader : public ByteSource {
public:
    ExtentReader(const QStringList &files, const QVector<Extent> &extents);

    int64_t size() const override { return total_; }
    int read(uint8_t *buf, int want) override;
    int64_t seek(int64_t pos) override;
    void stop() override;
    int64_t badBlocks() const override { return bad_.load(); }
    bool discLost() const override { return lost_.load(); }

    // Where a read lands, for tests: the extent index and offset within it.
    bool locate(int64_t pos, int *extent, int64_t *within) const;

private:
    int readOnce(uint8_t *buf, int want);
    bool fileGone(int index) const;

    QStringList names_;
    QVector<Extent> extents_;
    std::vector<int64_t> starts_; // cumulative start of each extent
    std::vector<std::unique_ptr<QFile>> files_;
    int64_t total_ = 0;
    int64_t pos_ = 0;
    std::atomic<bool> stopped_{false};
    std::atomic<int64_t> bad_{0};
    std::atomic<bool> lost_{false};
    int failedInARow_ = 0;
#ifdef _WIN32
    std::atomic<void *> readingThread_{nullptr};
#endif
};

} // namespace wmp
