// Where the sound comes from on the desktop. The audio thread copies what it's handing to
// the device into a ring, and the picture reads the newest of it whenever it draws. There's
// no lock, and the audio thread never waits, so it can't add latency or a glitch: a slow
// picture only means older samples get overwritten. It costs one atomic read while off.
#pragma once

#include "wvanalysis.h"

#include <atomic>
#include <cstdint>
#include <cstring>
#include <vector>

namespace wv {

class SampleTap {
public:
    SampleTap() : ring_(size_t(kCapacity) * 2, 0) {}

    // Audio thread: interleaved 16-bit stereo, as it goes to the device.
    void write(const int16_t *frames, int count)
    {
        if (!enabled_.load(std::memory_order_relaxed) || count <= 0)
            return;
        uint64_t head = head_.load(std::memory_order_relaxed);
        for (int done = 0; done < count;) {
            const int at = int(head % kCapacity);
            const int run = std::min(count - done, kCapacity - at);
            std::memcpy(ring_.data() + size_t(at) * 2, frames + size_t(done) * 2, size_t(run) * 2 * sizeof(int16_t));
            done += run;
            head += uint64_t(run);
        }
        head_.store(head, std::memory_order_release);
    }

    void setEnabled(bool on) { enabled_.store(on, std::memory_order_relaxed); }
    bool enabled() const { return enabled_.load(std::memory_order_relaxed); }
    void setRate(int rate) { rate_.store(rate, std::memory_order_relaxed); }
    int rate() const { return rate_.load(std::memory_order_relaxed); }
    // The sound is handed over this far ahead of being heard, so the picture waits for it.
    void setDelayFrames(int frames) { delay_.store(frames, std::memory_order_relaxed); }

    // Fills both with the kFftSize frames being heard about now. False if nothing has come
    // in lately, which means something other than this path is making the sound.
    bool read(float *left, float *right) const
    {
        const uint64_t head = head_.load(std::memory_order_acquire);
        const uint64_t want = uint64_t(kFftSize) + uint64_t(delay_.load(std::memory_order_relaxed));
        const bool moving = head != lastHead_;
        if (moving)
            lastHead_ = head;
        if (head < want) {
            std::fill(left, left + kFftSize, 0.f);
            std::fill(right, right + kFftSize, 0.f);
            return moving;
        }
        const uint64_t from = head - want;
        for (int i = 0; i < kFftSize; i++) {
            const size_t at = size_t((from + uint64_t(i)) % kCapacity) * 2;
            left[i] = float(ring_[at]) / 32768.f;
            right[i] = float(ring_[at + 1]) / 32768.f;
        }
        return moving;
    }

private:
    static constexpr int kCapacity = 1 << 15; // over half a second at any common rate
    std::vector<int16_t> ring_;
    std::atomic<uint64_t> head_{0};
    std::atomic<bool> enabled_{false};
    std::atomic<int> rate_{48000};
    std::atomic<int> delay_{0};
    mutable uint64_t lastHead_ = 0; // the reader's own
};

} // namespace wv
