// Listening: analysis.ts in C++. They're kept the same on purpose, and a test holds both to
// tests/fixtures/analysis-golden.json, so a change to one goes to the other.
#pragma once

#include <array>
#include <cstdint>
#include <vector>

namespace wv {

constexpr int kFftSize = 2048;
constexpr int kSpectrumBins = 512;
constexpr int kWavePoints = 512;
constexpr int kHistoryRows = 64;
constexpr int kAudioRows = 4 + kHistoryRows;
constexpr int kHistoryRate = 64; // rows a second, so they hold one second on any screen
constexpr double kMinBeatGap = 0.24;

struct AudioFrame {
    std::vector<uint8_t> texture; // kSpectrumBins wide, kAudioRows high
    int historyRow = 0;
    int historyMoved = 0; // how many rows that is on from the last frame
    double bass = 0, mid = 0, treble = 0, level = 0;
    double beat = 0;
    int beatCount = 0;
    std::array<double, 4> beatAges{60, 60, 60, 60};
    bool isSilent = true;
};

class Analyzer {
public:
    Analyzer();
    // left and right hold kFftSize samples, the newest last.
    void process(const float *left, const float *right, int rate, double dtSec);
    const AudioFrame &frame() const { return frame_; }

    double sensitivity = 1;
    double minBeatGap = kMinBeatGap;

private:
    struct Band {
        double value = 0, peak = 1e-4;
        void update(double power, double dt, double sensitivity);
    };
    void setRate(int rate);
    double bandPower(double fromHz, double toHz) const;
    void bands(double dt);
    void spectrumRow(double dt, bool silent);
    void waves(const float *left, const float *right);
    void detectBeat(double dt);
    void fft();

    AudioFrame frame_;
    std::vector<double> cos_, sin_, window_, re_, im_, mono_, mags_, smooth_, peaks_, binLo_, binHi_;
    std::vector<uint32_t> rev_;
    Band bass_, mid_, treble_, level_;
    std::vector<double> lows_, lowsAt_;
    int lowsHead_ = 0;
    double historyDue_ = 0;
    double lastLow_ = 0, lastLowInput_ = 0, clock_ = 0, lastBeat_ = -10, specPeak_ = 1e-3;
    int rate_ = 0;
    std::array<double, 4> beats_{60, 60, 60, 60};
};

} // namespace wv
