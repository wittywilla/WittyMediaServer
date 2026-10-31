#include "cdstream.h"

#include "engine/engineclient.h"

#include <QFileInfo>
#include <QProcess>

namespace wmp {

namespace {
constexpr int kMaxRead = 256 * 1024;
constexpr int kWaitMs = 100; // how often a wait looks at the stop flag
} // namespace

CdStream::CdStream(const QString &source) : source_(source) {}

CdStream::~CdStream() = default;

bool CdStream::begin(QString *why)
{
    proc_ = std::make_unique<QProcess>();
    proc_->setStandardErrorFile(QProcess::nullDevice());
    proc_->start(EngineClient::enginePath(), {QStringLiteral("--cd-stream"), source_}, QIODevice::ReadWrite);
    if (!proc_->waitForStarted(10000)) {
        *why = QStringLiteral("WittyMediaPlayer's helper program didn't start, so this CD can't be read.");
        finishedReading();
        return false;
    }
    QByteArray first;
    if (!line(&first) || !first.startsWith("S ")) {
        *why = first.startsWith("E ") ? QString::fromUtf8(first.mid(2)) : QStringLiteral("This CD couldn't be read.");
        if (!why->isEmpty())
            (*why)[0] = why->at(0).toUpper();
        finishedReading();
        return false;
    }
    size_ = first.mid(2).trimmed().toLongLong();
    return size_ > 0;
}

void CdStream::finishedReading()
{
    if (!proc_)
        return;
    proc_->closeWriteChannel(); // it ends when its input does
    if (!proc_->waitForFinished(2000))
        proc_->kill();
    proc_->waitForFinished(1000);
    proc_.reset();
}

bool CdStream::line(QByteArray *out)
{
    while (!proc_->canReadLine()) {
        if (stopped_.load())
            return false;
        if (!proc_->waitForReadyRead(kWaitMs) && proc_->state() == QProcess::NotRunning && !proc_->canReadLine())
            return false;
    }
    *out = proc_->readLine(4096).trimmed();
    return true;
}

bool CdStream::bytes(char *out, int n)
{
    int got = 0;
    while (got < n) {
        const qint64 r = proc_->read(out + got, n - got);
        if (r < 0)
            return false;
        got += int(r);
        if (got == n)
            break;
        if (stopped_.load())
            return false;
        if (!proc_->waitForReadyRead(kWaitMs) && proc_->state() == QProcess::NotRunning && proc_->bytesAvailable() == 0)
            return false;
    }
    return true;
}

int CdStream::read(uint8_t *buf, int want)
{
    if (stopped_.load() || !proc_)
        return -1;
    if (pos_ >= size_)
        return 0;
    const int n = qMin(want, kMaxRead);
    proc_->write(QStringLiteral("R %1 %2\n").arg(pos_).arg(n).toLatin1());
    proc_->waitForBytesWritten(kWaitMs);
    QByteArray head;
    if (!line(&head))
        return stopped_.load() ? -1 : 0;
    if (head.startsWith("E ")) {
        lost_.store(true);
        return 0;
    }
    const int got = head.startsWith("D ") ? head.mid(2).toInt() : -1;
    if (got < 0 || got > n || !bytes(reinterpret_cast<char *>(buf), got))
        return stopped_.load() ? -1 : 0;
    pos_ += got;
    return got;
}

int64_t CdStream::seek(int64_t pos)
{
    if (pos < 0)
        return -1;
    pos_ = pos;
    return pos;
}

} // namespace wmp
