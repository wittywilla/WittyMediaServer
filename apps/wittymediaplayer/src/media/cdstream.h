// An audio CD as one seekable WAV for the decoder. The helper program does the reading,
// from a drive or an image, one request at a time; everything on this side happens on
// the demux thread, which also creates and ends the process.
#pragma once

#include "extentreader.h"

#include <QString>
#include <atomic>
#include <memory>

class QProcess;

namespace wmp {

class CdStream : public ByteSource {
public:
    explicit CdStream(const QString &source);
    ~CdStream() override;

    // Starts the helper and learns the size. On the reading thread.
    bool begin(QString *why);

    int64_t size() const override { return size_; }
    int read(uint8_t *buf, int want) override;
    int64_t seek(int64_t pos) override;
    void stop() override { stopped_.store(true); }
    bool discLost() const override { return lost_.load(); }
    void finishedReading() override;

private:
    bool line(QByteArray *out);
    bool bytes(char *out, int n);

    QString source_;
    std::unique_ptr<QProcess> proc_;
    int64_t size_ = 0;
    int64_t pos_ = 0;
    std::atomic<bool> stopped_{false};
    std::atomic<bool> lost_{false};
};

} // namespace wmp
