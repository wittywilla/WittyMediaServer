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

class ExtentReader {
public:
    ExtentReader(const QStringList &files, const QVector<Extent> &extents);
    ~ExtentReader();

    int64_t size() const { return total_; }
    int read(uint8_t *buf, int want); // bytes read, 0 at the end, -1 when stopped
    int64_t seek(int64_t pos);

    // Stop any read in progress and refuse further ones. Safe from another thread.
    void stop();

    // Skipped because they wouldn't read.
    int64_t badBlocks() const { return bad_.load(); }
    bool discLost() const { return lost_.load(); }

    // A context for FFmpeg, owned by this reader.
    AVIOContext *avio();

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
    AVIOContext *avio_ = nullptr;
    std::atomic<bool> stopped_{false};
    std::atomic<int64_t> bad_{0};
    std::atomic<bool> lost_{false};
    int failedInARow_ = 0;
#ifdef _WIN32
    std::atomic<void *> readingThread_{nullptr};
#endif
};

} // namespace wmp
