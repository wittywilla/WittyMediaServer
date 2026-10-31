#include "wvanalysis.h"

#include <algorithm>
#include <cmath>

namespace wv {

namespace {
constexpr double kLowHz = 40, kHighHz = 16000, kPi = 3.14159265358979323846;
double clamp01(double v) { return v < 0 ? 0 : v > 1 ? 1 : v; }
uint8_t byteOf(double v) { return uint8_t(std::floor(clamp01(v) * 255 + 0.5)); }
} // namespace

void Analyzer::Band::update(double power, double dt, double sensitivity)
{
    const double amp = std::sqrt(power);
    peak = std::max({amp, peak * std::exp(-dt / 6), 0.0015});
    const double target = clamp01((amp / peak) * sensitivity);
    const double tau = target > value ? 0.02 : 0.2;
    value += (target - value) * (1 - std::exp(-dt / tau));
}

Analyzer::Analyzer()
{
    const int n = kFftSize;
    cos_.resize(n / 2);
    sin_.resize(n / 2);
    for (int i = 0; i < n / 2; i++) {
        cos_[i] = std::cos(2 * kPi * i / n);
        sin_[i] = std::sin(2 * kPi * i / n);
    }
    const int bits = int(std::log2(n) + 0.5);
    rev_.resize(n);
    for (int i = 0; i < n; i++) {
        uint32_t r = 0;
        for (int b = 0; b < bits; b++)
            if (i & (1 << b))
                r |= 1u << (bits - 1 - b);
        rev_[i] = r;
    }
    window_.resize(n);
    for (int i = 0; i < n; i++)
        window_[i] = 0.5 - 0.5 * std::cos(2 * kPi * i / (n - 1));
    re_.resize(n);
    im_.resize(n);
    mono_.resize(n);
    mags_.resize(n / 2);
    smooth_.resize(kSpectrumBins);
    peaks_.resize(kSpectrumBins);
    binLo_.resize(kSpectrumBins);
    binHi_.resize(kSpectrumBins);
    lows_.assign(128, 0);
    lowsAt_.assign(128, -100);
    frame_.texture.assign(size_t(kSpectrumBins) * kAudioRows, 0);
}

void Analyzer::setRate(int rate)
{
    rate_ = rate;
    const double hzPerBin = double(rate) / kFftSize;
    const double top = std::min(kHighHz, rate * 0.48);
    for (int i = 0; i < kSpectrumBins; i++) {
        binLo_[i] = (kLowHz * std::pow(top / kLowHz, double(i) / kSpectrumBins)) / hzPerBin;
        binHi_[i] = (kLowHz * std::pow(top / kLowHz, double(i + 1) / kSpectrumBins)) / hzPerBin;
    }
}

void Analyzer::fft()
{
    const int n = kFftSize;
    for (int i = 0; i < n; i++) {
        const uint32_t j = rev_[i];
        if (j > uint32_t(i)) {
            std::swap(re_[i], re_[j]);
            std::swap(im_[i], im_[j]);
        }
    }
    for (int size = 2; size <= n; size *= 2) {
        const int half = size / 2, step = n / size;
        for (int start = 0; start < n; start += size) {
            for (int k = 0; k < half; k++) {
                const int w = k * step, a = start + k, b = a + half;
                const double tr = re_[b] * cos_[w] + im_[b] * sin_[w];
                const double ti = im_[b] * cos_[w] - re_[b] * sin_[w];
                re_[b] = re_[a] - tr;
                im_[b] = im_[a] - ti;
                re_[a] += tr;
                im_[a] += ti;
            }
        }
    }
}

void Analyzer::process(const float *left, const float *right, int rate, double dtSec)
{
    if (rate != rate_)
        setRate(rate);
    const double dt = std::max(0.001, std::min(0.25, dtSec));
    clock_ += dt;
    double energy = 0;
    for (int i = 0; i < kFftSize; i++) {
        const double m = 0.5 * (double(left[i]) + double(right[i]));
        mono_[i] = m;
        energy += m * m;
        re_[i] = m * window_[i];
        im_[i] = 0;
    }
    fft();
    const double norm = 4.0 / kFftSize;
    for (int i = 0; i < kFftSize / 2; i++)
        mags_[i] = std::hypot(re_[i], im_[i]) * norm;
    frame_.isSilent = energy / kFftSize < 1e-8;

    bands(dt);
    spectrumRow(dt, frame_.isSilent);
    waves(left, right);
    detectBeat(dt);

    frame_.bass = bass_.value;
    frame_.mid = mid_.value;
    frame_.treble = treble_.value;
    frame_.level = level_.value;
    frame_.beat *= std::exp(-dt / 0.16);
    for (int i = 0; i < 4; i++) {
        beats_[i] += dt;
        frame_.beatAges[i] = beats_[i];
    }
}

double Analyzer::bandPower(double fromHz, double toHz) const
{
    const double per = double(rate_) / kFftSize;
    const int a = std::max(1, int(std::floor(fromHz / per + 0.5)));
    const int b = std::min(kFftSize / 2 - 1, int(std::floor(toHz / per + 0.5)));
    double p = 0;
    for (int i = a; i <= b; i++)
        p += mags_[i] * mags_[i];
    return p;
}

void Analyzer::bands(double dt)
{
    const double bass = bandPower(20, 180), mid = bandPower(180, 2000), treble = bandPower(2000, 16000);
    bass_.update(bass, dt, sensitivity);
    mid_.update(mid, dt, sensitivity);
    treble_.update(treble, dt, sensitivity);
    level_.update(bass + mid + treble, dt, sensitivity);
    lastLowInput_ = bass;
}

void Analyzer::spectrumRow(double dt, bool silent)
{
    const int w = kSpectrumBins;
    auto &row = frame_.texture;
    const double fall = 1 - std::exp(-dt / 0.14);
    double top = 0;
    for (int i = 0; i < w; i++) {
        const double lo = binLo_[i], hi = binHi_[i];
        double v = 0;
        if (hi - lo < 1) {
            const double c = (lo + hi) * 0.5;
            const int i0 = int(std::floor(c));
            const double t = c - i0;
            v = mags_[i0] * (1 - t) + mags_[std::min(i0 + 1, kFftSize / 2 - 1)] * t;
        } else {
            for (int k = int(std::floor(lo)); k < std::min(int(std::ceil(hi)), kFftSize / 2); k++)
                v = std::max(v, mags_[k]);
        }
        const double octave = double(i) / w * std::log2(kHighHz / kLowHz);
        v *= std::pow(2.0, octave * 0.5);
        top = std::max(top, v);
        re_[i] = v;
    }
    specPeak_ = std::max({top, specPeak_ * std::exp(-dt / 8), 0.002});
    const double rangeDb = 52;
    frame_.historyRow = (frame_.historyRow + 1) % kHistoryRows;
    const size_t histAt = size_t(4 + frame_.historyRow) * w;
    for (int i = 0; i < w; i++) {
        const double db = silent ? -120 : 20 * std::log10(std::max(re_[i], 1e-9) / specPeak_);
        const double target = clamp01(1 + db / rangeDb) * std::min(1.15, sensitivity);
        double s = smooth_[i];
        s = target > s ? target : s + (target - s) * fall;
        smooth_[i] = s;
        peaks_[i] = std::max(s, peaks_[i] - 0.35 * dt);
        const uint8_t b = byteOf(s);
        row[i] = b;
        row[size_t(3) * w + i] = byteOf(peaks_[i]);
        row[histAt + i] = b;
    }
}

void Analyzer::waves(const float *left, const float *right)
{
    auto &row = frame_.texture;
    const int w = kSpectrumBins, span = kWavePoints * 2;
    int start = kFftSize - span;
    for (int i = kFftSize - span; i > 1 && i > kFftSize - span - 600; i--) {
        if (mono_[i - 1] <= 0 && mono_[i] > 0) {
            start = i;
            break;
        }
    }
    for (int i = 0; i < kWavePoints; i++) {
        const int at = start + i * 2;
        row[size_t(w) + i] = byteOf(double(left[at]) * 0.5 + 0.5);
        row[size_t(2) * w + i] = byteOf(double(right[at]) * 0.5 + 0.5);
    }
}

void Analyzer::detectBeat(double dt)
{
    const double low = std::log10(lastLowInput_ + 1e-6) * 10;
    const double jump = std::max(0.0, low - lastLow_);
    lastLow_ += (low - lastLow_) * (1 - std::exp(-dt / 0.03));
    int n = 0;
    double sum = 0, sq = 0;
    for (size_t i = 0; i < lows_.size(); i++) {
        if (clock_ - lowsAt_[i] > 1.5)
            continue;
        n++;
        sum += lows_[i];
        sq += lows_[i] * lows_[i];
    }
    const double mean = n ? sum / n : 0;
    const double spread = n ? std::sqrt(std::max(0.0, sq / n - mean * mean)) : 0;
    lows_[lowsHead_] = jump;
    lowsAt_[lowsHead_] = clock_;
    lowsHead_ = (lowsHead_ + 1) % int(lows_.size());
    const double threshold = mean + (1.8 / sensitivity) * spread + 1.5 / sensitivity;
    if (jump > threshold && low > -48 && n > 6 && clock_ - lastBeat_ >= minBeatGap) {
        lastBeat_ = clock_;
        frame_.beat = 1;
        frame_.beatCount++;
        beats_ = {0, beats_[0], beats_[1], beats_[2]};
    }
}

} // namespace wv
