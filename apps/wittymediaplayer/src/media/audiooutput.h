// Sound output: a ring of decoded samples the system's audio device pulls from, in
// a thread of its own. It is also the clock the picture follows.
#pragma once

#include <QByteArray>
#include <QObject>
#include <QThread>
#include <QtGlobal>
#include <atomic>
#include <chrono>
#include <condition_variable>
#include <deque>
#include <mutex>
#include <vector>

class QAudioSink;
class QTimer;

namespace wmp {

class AudioOutput : public QObject {
    Q_OBJECT
public:
    explicit AudioOutput(QObject *parent = nullptr);
    ~AudioOutput() override;

    // What decoders should give it: 16-bit interleaved stereo at this rate.
    int sampleRate() const { return rate_.load(); }
    static constexpr int kChannels = 2;
    static constexpr int kBytesPerFrame = 4;

    // Starts the device thread. noDevice runs on a timer instead of real hardware,
    // for tests and machines with no sound card.
    void start(bool noDevice);
    bool hasDevice() const { return device_.load(); }

    // Waits for room, unless stop() or clear() comes first. Returns false then.
    bool write(const char *data, qsizetype bytes, double pts, uint64_t generation);
    // Drops everything queued, for a seek. Writes from older generations are refused.
    void clear(uint64_t generation);
    void setPaused(bool paused);
    void setVolume(double v);
    void stop();

    // The time of the sound being heard now. Negative when none is playing.
    double positionSec() const;
    // Seconds of sound waiting to be heard.
    double queuedSec() const;
    int64_t framesWritten() const { return framesWritten_.load(); }
    int64_t underruns() const { return underruns_.load(); }

    // Called by the device side.
    qint64 pull(char *out, qint64 max);

signals:
    void startRequested(bool noDevice);
    void pauseRequested(bool paused);
    void volumeRequested(double v);
    void stopRequested();

private slots:
    void onStart(bool noDevice);
    void onPause(bool paused);
    void onVolume(double v);
    void onStop();
    void onTick();

private:
    struct Mark {
        uint64_t at; // total bytes written when this time begins
        double pts;
    };

    QThread thread_;
    std::atomic<bool> device_{false};
    std::atomic<int> rate_{48000};
    std::atomic<bool> stopped_{false};
    std::atomic<int64_t> framesWritten_{0};
    std::atomic<int64_t> underruns_{0};

    mutable std::mutex mu_;
    std::condition_variable room_;
    std::vector<char> ring_;
    size_t head_ = 0, count_ = 0;
    uint64_t written_ = 0, read_ = 0; // bytes through the ring, real sound only
    std::deque<Mark> marks_;
    uint64_t generation_ = 0;
    uint64_t served_ = 0;    // bytes given to the device, silence included
    uint64_t realInDevice_ = 0; // of those, how far the real sound reaches
    bool paused_ = false;
    bool idle_ = true;

    // The position, kept fresh by the device thread and read from anywhere.
    mutable std::mutex posMu_;
    double anchorPos_ = -1;
    std::chrono::steady_clock::time_point anchorAt_{};
    bool moving_ = false;

    // Device-thread only.
    QAudioSink *sink_ = nullptr;
    QObject *ioDevice_ = nullptr;
    QTimer *tick_ = nullptr;
    qint64 resumedAtUs_ = 0;
    std::chrono::steady_clock::time_point nullLast_{};
    double volume_ = 1.0;
    bool noDevice_ = false;
    uint64_t lastRead_ = 0;
    int stalledTicks_ = 0;

    double ptsOf(uint64_t byte) const;
};

} // namespace wmp
