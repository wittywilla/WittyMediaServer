// Where the sound comes from on the desktop. The audio thread copies what it's handing to
// the device into a ring, and the picture reads the newest of it whenever it draws. There's
// no lock, and the audio thread never waits, so it can't add latency or a glitch: a slow
// picture only means older samples get overwritten. It costs one atomic read while off.
#pragma once

#include "wvanalysis.h"

#include <algorithm>
#include <atomic>
#include <chrono>
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
        if (!enabled() || count <= 0)
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

    // On while anyone wants it: the visualizer is user 1.
    void setUser(int bit, bool on)
    {
        if (on)
            users_.fetch_or(bit, std::memory_order_relaxed);
        else
            users_.fetch_and(~bit, std::memory_order_relaxed);
    }
    void setEnabled(bool on) { setUser(1, on); }
    bool enabled() const { return users_.load(std::memory_order_relaxed) != 0; }
    uint64_t written() const { return head_.load(std::memory_order_acquire); }
    void setRate(int rate) { rate_.store(rate, std::memory_order_relaxed); }
    int rate() const { return rate_.load(std::memory_order_relaxed); }
    // The sound is handed over this far ahead of being heard, so the picture waits for it.
    void setDelayFrames(int frames) { delay_.store(frames, std::memory_order_relaxed); }

    // The newest frames, interleaved, oldest first, leaving out what's still to be heard.
    void readInterleaved(int16_t *out, int frames, int behind) const
    {
        const uint64_t head = head_.load(std::memory_order_acquire);
        const uint64_t want = uint64_t(frames) + uint64_t(behind);
        for (int i = 0; i < frames; i++) {
            if (head + uint64_t(i) < want) {
                out[size_t(i) * 2] = out[size_t(i) * 2 + 1] = 0;
                continue;
            }
            const size_t at = size_t((head + uint64_t(i) - want) % kCapacity) * 2;
            out[size_t(i) * 2] = ring_[at];
            out[size_t(i) * 2 + 1] = ring_[at + 1];
        }
    }

    // Fills both with the kFftSize frames being heard about now. A device that has stopped
    // pulling (a pause) reads as silence. False if nothing new has come in, and if that goes on
    // from the start, something other than this path is making the sound.
    bool read(float *left, float *right) const
    {
        const uint64_t head = head_.load(std::memory_order_acquire);
        const uint64_t want = uint64_t(kFftSize) + uint64_t(delay_.load(std::memory_order_relaxed));
        const bool moving = head != lastHead_;
        const auto now = std::chrono::steady_clock::now();
        if (moving) {
            lastHead_ = head;
            lastMoved_ = now;
        }
        if (head < want || now - lastMoved_ > std::chrono::milliseconds(300)) {
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

    static constexpr int kCapacity = 1 << 15; // over half a second at any common rate

private:
    std::vector<int16_t> ring_;
    std::atomic<uint64_t> head_{0};
    std::atomic<int> users_{0};
    std::atomic<int> rate_{48000};
    std::atomic<int> delay_{0};
    mutable uint64_t lastHead_ = 0; // these two are the reader's own
    mutable std::chrono::steady_clock::time_point lastMoved_{};
};

} // namespace wv
