#include "audiooutput.h"

#include <QAudioDevice>
#include <QAudioFormat>
#include <QAudioSink>
#include <QCoreApplication>
#include <QIODevice>
#include <QMediaDevices>
#include <QTimer>
#include <algorithm>
#include <cstring>

namespace wmp {

namespace {
constexpr double kRingSeconds = 0.75;

class Pull : public QIODevice {
public:
    explicit Pull(AudioOutput *o) : output_(o) { open(QIODevice::ReadOnly); }
    bool isSequential() const override { return true; }
    qint64 bytesAvailable() const override { return 65536 + QIODevice::bytesAvailable(); }
protected:
    qint64 readData(char *data, qint64 max) override { return output_->pull(data, max); }
    qint64 writeData(const char *, qint64) override { return -1; }
private:
    AudioOutput *output_;
};
} // namespace

AudioOutput::AudioOutput(QObject *parent) : QObject(parent)
{
    connect(this, &AudioOutput::startRequested, this, &AudioOutput::onStart, Qt::QueuedConnection);
    connect(this, &AudioOutput::pauseRequested, this, &AudioOutput::onPause, Qt::QueuedConnection);
    connect(this, &AudioOutput::volumeRequested, this, &AudioOutput::onVolume, Qt::QueuedConnection);
    connect(this, &AudioOutput::stopRequested, this, &AudioOutput::onStop, Qt::BlockingQueuedConnection);
}

AudioOutput::~AudioOutput()
{
    stop();
}

void AudioOutput::start(bool noDevice)
{
    // The rate has to be known before the first frame is decoded.
    if (!noDevice) {
        const QAudioDevice dev = QMediaDevices::defaultAudioOutput();
        if (dev.isNull()) {
            noDevice = true;
        } else {
            QAudioFormat f = dev.preferredFormat();
            if (f.sampleRate() > 0)
                rate_.store(std::clamp(f.sampleRate(), 8000, 192000));
        }
    }
    ring_.assign(size_t(rate_.load() * kRingSeconds) * kBytesPerFrame, 0);
    moveToThread(&thread_);
    thread_.setObjectName(QStringLiteral("audio"));
    thread_.start(QThread::TimeCriticalPriority);
    emit startRequested(noDevice);
}

void AudioOutput::stop()
{
    if (stopped_.exchange(true))
        return;
    {
        std::lock_guard<std::mutex> l(mu_);
        room_.notify_all();
    }
    if (thread_.isRunning()) {
        emit stopRequested();
        thread_.quit();
        thread_.wait();
    }
}

void AudioOutput::onStart(bool noDevice)
{
    tick_ = new QTimer(this);
    tick_->setTimerType(Qt::PreciseTimer);
    tick_->setInterval(20);
    connect(tick_, &QTimer::timeout, this, &AudioOutput::onTick);
    noDevice_ = noDevice;
    nullLast_ = std::chrono::steady_clock::now();
    if (!noDevice) {
        const QAudioDevice dev = QMediaDevices::defaultAudioOutput();
        QAudioFormat f;
        f.setSampleRate(rate_.load());
        f.setChannelCount(kChannels);
        f.setSampleFormat(QAudioFormat::Int16);
        if (!dev.isFormatSupported(f)) {
            noDevice_ = true;
        } else {
            sink_ = new QAudioSink(dev, f, this);
            // Small enough to answer a pause at once, big enough to ride out a busy machine.
            sink_->setBufferSize(rate_.load() / 5 * kBytesPerFrame);
            sink_->setVolume(volume_);
            auto *io = new Pull(this);
            io->setParent(this);
            ioDevice_ = io;
            sink_->start(io);
            if (sink_->error() != QAudio::NoError) {
                delete sink_;
                sink_ = nullptr;
                noDevice_ = true;
            } else {
                device_.store(true);
            }
        }
    }
    tick_->start();
}

void AudioOutput::onStop()
{
    if (tick_)
        tick_->stop();
    if (sink_) {
        sink_->stop();
        delete sink_;
        sink_ = nullptr;
    }
    delete ioDevice_;
    ioDevice_ = nullptr;
    moveToThread(QCoreApplication::instance() ? QCoreApplication::instance()->thread() : nullptr);
}

void AudioOutput::onPause(bool paused)
{
    {
        std::lock_guard<std::mutex> l(mu_);
        paused_ = paused;
    }
    if (sink_) {
        if (paused)
            sink_->suspend();
        else
            sink_->resume();
    }
    nullLast_ = std::chrono::steady_clock::now();
    onTick();
}

void AudioOutput::setPaused(bool paused)
{
    emit pauseRequested(paused);
}

void AudioOutput::onVolume(double v)
{
    volume_ = v;
    if (sink_)
        sink_->setVolume(v);
}

void AudioOutput::setVolume(double v)
{
    emit volumeRequested(v);
}

void AudioOutput::wake()
{
    std::lock_guard<std::mutex> l(mu_);
    room_.notify_all();
}

bool AudioOutput::write(const char *data, qsizetype bytes, double pts, uint64_t generation, const std::atomic<bool> *abort)
{
    std::unique_lock<std::mutex> l(mu_);
    bool first = true;
    while (bytes > 0) {
        // Paused, nothing drains the ring: a session ending has to be able to get out.
        room_.wait(l, [&] { return stopped_.load() || abort->load() || generation != generation_ || count_ < ring_.size(); });
        if (stopped_.load() || abort->load() || generation != generation_)
            return false;
        if (first) {
            marks_.push_back({written_, pts});
            while (marks_.size() > 512)
                marks_.pop_front();
            first = false;
        }
        size_t tail = (head_ + count_) % ring_.size();
        size_t take = std::min({size_t(bytes), ring_.size() - count_, ring_.size() - tail});
        std::memcpy(ring_.data() + tail, data, take);
        count_ += take;
        written_ += take;
        data += take;
        bytes -= qsizetype(take);
    }
    return true;
}

void AudioOutput::clear(uint64_t generation)
{
    {
        std::lock_guard<std::mutex> l(mu_);
        generation_ = generation;
        head_ = count_ = 0;
        written_ = read_ = 0;
        served_ = realInDevice_ = 0;
        marks_.clear();
        idle_ = true;
    }
    {
        std::lock_guard<std::mutex> p(posMu_);
        anchorPos_ = -1;
        moving_ = false;
    }
    room_.notify_all();
}

qint64 AudioOutput::pull(char *out, qint64 max)
{
    std::lock_guard<std::mutex> l(mu_);
    qint64 done = 0;
    qint64 want = max - max % kBytesPerFrame;
    while (done < want && count_ > 0) {
        size_t take = std::min({size_t(want - done), count_, ring_.size() - head_});
        std::memcpy(out + done, ring_.data() + head_, take);
        head_ = (head_ + take) % ring_.size();
        count_ -= take;
        read_ += take;
        done += qint64(take);
    }
    framesWritten_.fetch_add(done / kBytesPerFrame);
    if (done > 0) {
        idle_ = false;
        room_.notify_all();
    }
    // The device keeps running through a gap: silence, not an underrun it must
    // recover from, which would cost a click and a restart on some drivers.
    if (done < want) {
        std::memset(out + done, 0, size_t(want - done));
        if (!idle_)
            underruns_.fetch_add(1);
    }
    served_ += uint64_t(want);
    return want;
}

double AudioOutput::ptsOf(uint64_t byte) const
{
    const double bps = double(rate_.load()) * kBytesPerFrame;
    if (marks_.empty())
        return -1;
    const Mark *best = &marks_.front();
    for (const Mark &m : marks_) {
        if (m.at > byte)
            break;
        best = &m;
    }
    return best->pts + double(int64_t(byte) - int64_t(best->at)) / bps;
}

void AudioOutput::onTick()
{
    using clock = std::chrono::steady_clock;
    const double bps = double(rate_.load()) * kBytesPerFrame;
    double pos = -1;
    bool moving = false;
    {
        std::lock_guard<std::mutex> l(mu_);
        if (noDevice_ && !paused_) {
            // No sound card: consume sound in real time.
            auto now = clock::now();
            size_t take = size_t(std::chrono::duration<double>(now - nullLast_).count() * bps);
            take -= take % kBytesPerFrame;
            nullLast_ = now;
            take = std::min(take, count_);
            head_ = ring_.empty() ? 0 : (head_ + take) % ring_.size();
            count_ -= take;
            read_ += take;
            framesWritten_.fetch_add(qint64(take) / kBytesPerFrame);
            if (take > 0) {
                idle_ = false;
                room_.notify_all();
            }
        }
        uint64_t heard = read_;
        if (sink_) {
            // What the device still holds hasn't been heard yet. It may hold silence
            // from a gap, which counts for nothing, so this never reaches past read_.
            qint64 held = qMax<qint64>(0, sink_->bufferSize() - sink_->bytesFree());
            heard = read_ > uint64_t(held) ? read_ - uint64_t(held) : 0;
        }
        if (!marks_.empty() && read_ > 0)
            pos = ptsOf(heard);
        moving = !paused_ && count_ > 0;
        // A short gap keeps going; a long one means the sound has stopped for now,
        // and the picture must not wait for it.
        if (read_ == lastRead_ && !paused_) {
            if (++stalledTicks_ > 25)
                pos = -1;
        } else {
            stalledTicks_ = 0;
            lastRead_ = read_;
        }
        if (read_ > 0 && !paused_)
            moving = stalledTicks_ < 5;
    }
    std::lock_guard<std::mutex> p(posMu_);
    anchorPos_ = pos;
    anchorAt_ = clock::now();
    moving_ = moving;
}

double AudioOutput::positionSec() const
{
    std::lock_guard<std::mutex> p(posMu_);
    if (anchorPos_ < 0)
        return -1;
    if (!moving_)
        return anchorPos_;
    double extra = std::chrono::duration<double>(std::chrono::steady_clock::now() - anchorAt_).count();
    return anchorPos_ + std::min(extra, 0.1);
}

double AudioOutput::queuedSec() const
{
    std::lock_guard<std::mutex> l(mu_);
    return double(count_) / (double(rate_.load()) * kBytesPerFrame);
}

} // namespace wmp
