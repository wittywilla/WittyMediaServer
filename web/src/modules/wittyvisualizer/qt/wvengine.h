// What the visualizer decides, with no drawing in it: engine.ts in C++, and the limits from
// safety.ts. It reads the same effect files for their descriptions.
#pragma once

#include <QString>
#include <QStringList>
#include <QVector>
#include <array>
#include <functional>
#include <random>

class QSettings;

namespace wv {

// shaders/passes.glsl does the sums; these are the limits it's given (safety.ts).
struct GuardLimits {
    float deadBand, rise;
};
constexpr GuardLimits kGuard{0.04f, 0.15f};
constexpr GuardLimits kGuardCalm{0.02f, 0.05f};

struct Settings {
    bool notice = false;
    bool on = false;
    QString effect;
    bool random = true;
    int cycleSec = 30;
    QStringList favourites;
    bool favouritesOnly = false;
    double intensity = 1;
    double sensitivity = 1;
    int calm = -1; // -1 follows the system, 0 off, 1 on
    double scale = 0; // 0 adapts by itself
    bool stats = false;

    void load(QSettings &s);
    void save(QSettings &s) const;
};
QVector<int> cycleChoices();
QVector<double> scaleChoices();

struct EffectInfo {
    QString id, name, description;
    int cost = 2;
    bool gentle = false;
    bool feedback = false;
    int quads = 0;
};
// The effects built in, from the descriptions in their own files.
const QVector<EffectInfo> &builtinEffects();
// Reads one description. Empty id if it isn't one.
EffectInfo parseEffect(const QByteArray &file);

QStringList eligible(const QVector<EffectInfo> &all, const Settings &s, bool calm, bool weak);

class Cycle {
public:
    explicit Cycle(unsigned seed = std::random_device{}()) : rand_(seed) {}
    QString next(const QStringList &ids, bool random);
    QString previous(const QStringList &ids, bool random);
    void select(const QString &id);
    QString current;

private:
    QStringList queue_, past_;
    std::mt19937 rand_;
};

class Budget {
public:
    Budget(double start, double floor, double ceiling) : scale(start), floor_(floor), ceiling_(ceiling) {}
    // True when the resolution or frame rate changed.
    bool frame(double dt);
    double scale;
    double frameGap = 0;
    double fps = 0;
    double floor() const { return floor_; }

private:
    double floor_, ceiling_;
    double avg_ = 1.0 / 60, held_ = 0, calmFor_ = 0, countedFor_ = 0;
    int counted_ = 0;
};

struct PaletteDef {
    QString id;
    std::array<float, 3> a, b, c, d;
};
QVector<PaletteDef> builtinPalettes();

class Palettes {
public:
    explicit Palettes(const QVector<PaletteDef> &list, unsigned seed = std::random_device{}());
    void step(double dt, int beatCount, bool calm);
    std::array<std::array<float, 3>, 4> out; // a, b, c, d

private:
    void blend();
    QVector<PaletteDef> list_;
    int from_ = 0, to_ = 0;
    double t_ = 1, since_ = 0;
    int beatsAtChange_ = 0;
    std::mt19937 rand_;
};

// The system's "reduce motion" choice, where one can be read.
bool systemWantsLessMotion();

} // namespace wv
