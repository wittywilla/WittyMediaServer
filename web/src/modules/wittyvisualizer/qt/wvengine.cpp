#include "wvengine.h"

#include <QDir>
#include <QFile>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QSettings>
#include <algorithm>
#ifdef Q_OS_WIN
#include <windows.h>
#endif

namespace wv {

QVector<int> cycleChoices() { return {0, 10, 15, 30, 60, 120, 300}; }
QVector<double> scaleChoices() { return {0, 1, 0.75, 0.5, 0.33}; }

void Settings::load(QSettings &s)
{
    s.beginGroup(QStringLiteral("visualizer"));
    notice = s.value(QStringLiteral("notice"), false).toBool();
    on = s.value(QStringLiteral("on"), false).toBool();
    effect = s.value(QStringLiteral("effect")).toString().left(40);
    random = s.value(QStringLiteral("random"), true).toBool();
    const int c = s.value(QStringLiteral("cycleSec"), 30).toInt();
    cycleSec = cycleChoices().contains(c) ? c : 30;
    favourites = s.value(QStringLiteral("favourites")).toStringList().mid(0, 64);
    favouritesOnly = s.value(QStringLiteral("favouritesOnly"), false).toBool();
    intensity = std::clamp(s.value(QStringLiteral("intensity"), 1.0).toDouble(), 0.3, 1.5);
    sensitivity = std::clamp(s.value(QStringLiteral("sensitivity"), 1.0).toDouble(), 0.25, 2.0);
    calm = std::clamp(s.value(QStringLiteral("calm"), -1).toInt(), -1, 1);
    const double sc = s.value(QStringLiteral("scale"), 0.0).toDouble();
    scale = scaleChoices().contains(sc) ? sc : 0;
    stats = s.value(QStringLiteral("stats"), false).toBool();
    s.endGroup();
}

void Settings::save(QSettings &s) const
{
    s.beginGroup(QStringLiteral("visualizer"));
    s.setValue(QStringLiteral("notice"), notice);
    s.setValue(QStringLiteral("on"), on);
    s.setValue(QStringLiteral("effect"), effect);
    s.setValue(QStringLiteral("random"), random);
    s.setValue(QStringLiteral("cycleSec"), cycleSec);
    s.setValue(QStringLiteral("favourites"), favourites);
    s.setValue(QStringLiteral("favouritesOnly"), favouritesOnly);
    s.setValue(QStringLiteral("intensity"), intensity);
    s.setValue(QStringLiteral("sensitivity"), sensitivity);
    s.setValue(QStringLiteral("calm"), calm);
    s.setValue(QStringLiteral("scale"), scale);
    s.setValue(QStringLiteral("stats"), stats);
    s.endGroup();
}

EffectInfo parseEffect(const QByteArray &file)
{
    EffectInfo e;
    const QByteArray open("/*wittyvisualizer");
    const int at = file.indexOf(open), end = file.indexOf("*/");
    if (at < 0 || end < 0)
        return e;
    const QJsonObject o = QJsonDocument::fromJson(file.mid(at + open.size(), end - at - open.size())).object();
    if (o.value(QStringLiteral("format")).toInt() != 1)
        return e;
    e.id = o.value(QStringLiteral("id")).toString();
    e.name = o.value(QStringLiteral("name")).toString();
    e.description = o.value(QStringLiteral("description")).toString();
    e.cost = o.value(QStringLiteral("cost")).toInt(2);
    e.gentle = o.value(QStringLiteral("motion")).toString() == QStringLiteral("gentle");
    e.feedback = o.value(QStringLiteral("feedback")).toBool();
    e.quads = o.value(QStringLiteral("geometry")).toObject().value(QStringLiteral("quads")).toInt();
    return e;
}

const QVector<EffectInfo> &builtinEffects()
{
    static const QVector<EffectInfo> all = [] {
        QVector<EffectInfo> list;
        const QDir dir(QStringLiteral(":/wv/effects"));
        for (const QString &name : dir.entryList({QStringLiteral("*.glsl")}, QDir::Files)) {
            QFile f(dir.filePath(name));
            if (f.open(QIODevice::ReadOnly)) {
                EffectInfo e = parseEffect(f.readAll());
                if (!e.id.isEmpty())
                    list.push_back(e);
            }
        }
        std::sort(list.begin(), list.end(), [](const EffectInfo &a, const EffectInfo &b) { return a.name.localeAwareCompare(b.name) < 0; });
        return list;
    }();
    return all;
}

QStringList eligible(const QVector<EffectInfo> &all, const Settings &s, bool calm, bool weak)
{
    QVector<EffectInfo> list = all;
    const auto keep = [](const QVector<EffectInfo> &from, const std::function<bool(const EffectInfo &)> &ok) {
        QVector<EffectInfo> out;
        for (const EffectInfo &e : from)
            if (ok(e))
                out.push_back(e);
        return out;
    };
    if (calm)
        list = keep(list, [](const EffectInfo &e) { return e.gentle; });
    if (s.favouritesOnly && !s.favourites.isEmpty()) {
        const auto f = keep(list, [&](const EffectInfo &e) { return s.favourites.contains(e.id); });
        if (!f.isEmpty())
            list = f;
    }
    if (weak) {
        const auto cheap = keep(list, [](const EffectInfo &e) { return e.cost < 3; });
        if (!cheap.isEmpty())
            list = cheap;
    }
    if (list.isEmpty())
        list = all;
    QStringList ids;
    for (const EffectInfo &e : list)
        ids << e.id;
    return ids;
}

QString Cycle::next(const QStringList &ids, bool random)
{
    if (ids.isEmpty())
        return current;
    QString n;
    if (random) {
        QStringList kept;
        for (const QString &q : queue_)
            if (ids.contains(q))
                kept << q;
        queue_ = kept;
        if (queue_.isEmpty()) {
            queue_ = ids;
            queue_.removeAll(current);
            std::shuffle(queue_.begin(), queue_.end(), rand_);
        }
        n = queue_.isEmpty() ? ids.first() : queue_.takeFirst();
    } else {
        const int i = current.isEmpty() ? -1 : ids.indexOf(current);
        n = ids[(i + 1) % ids.size()];
    }
    if (!current.isEmpty()) {
        past_ << current;
        if (past_.size() > 32)
            past_.removeFirst();
    }
    current = n;
    return n;
}

QString Cycle::previous(const QStringList &ids, bool random)
{
    if (ids.isEmpty())
        return current;
    if (random) {
        while (!past_.isEmpty()) {
            const QString p = past_.takeLast();
            if (ids.contains(p) && p != current) {
                if (!current.isEmpty())
                    queue_.prepend(current);
                current = p;
                return p;
            }
        }
    }
    const int i = std::max<int>(0, current.isEmpty() ? 0 : ids.indexOf(current));
    current = ids[(i - 1 + ids.size()) % ids.size()];
    return current;
}

void Cycle::select(const QString &id)
{
    if (!current.isEmpty() && current != id)
        past_ << current;
    current = id;
}

bool Budget::frame(double dt)
{
    counted_++;
    countedFor_ += dt;
    if (countedFor_ >= 1) {
        fps = counted_ / countedFor_;
        counted_ = 0;
        countedFor_ = 0;
    }
    if (dt > 0.5)
        return false;
    avg_ += (dt - avg_) * 0.08;
    held_ += dt;
    if (held_ < 1.2)
        return false;
    // Judged against the rate it's aiming for: 30 a second on purpose isn't slow.
    const double aim = frameGap > 0 ? frameGap : 1.0 / 60;
    const bool slow = avg_ > aim * 1.33, fast = avg_ < aim * 1.11;
    if (slow) {
        // If it has just tried more pixels and that failed, it waits longer before the next try.
        if (calmFor_ == 0 && rose_)
            patience_ = std::min(120.0, patience_ * 2);
        rose_ = false;
        calmFor_ = 0;
        held_ = 0;
        if (scale > floor_ + 1e-3) {
            scale = std::max(floor_, scale * (avg_ > 1.0 / 25 ? 0.7 : 0.85));
            return true;
        }
        if (frameGap < 1.0 / 31) {
            frameGap = 1.0 / 30;
            return true;
        }
        return false;
    }
    if (fast) {
        calmFor_ += dt;
        if (calmFor_ > patience_ && scale < ceiling_ - 1e-3) {
            calmFor_ = 0;
            held_ = 0;
            rose_ = true;
            scale = std::min(ceiling_, scale * 1.12);
            return true;
        }
    }
    return false;
}

QVector<PaletteDef> builtinPalettes()
{
    QVector<PaletteDef> list;
    QFile f(QStringLiteral(":/wv/effects/palettes.json"));
    if (f.open(QIODevice::ReadOnly)) {
        const QJsonArray arr = QJsonDocument::fromJson(f.readAll()).object().value(QStringLiteral("palettes")).toArray();
        for (const auto &v : arr) {
            const QJsonObject o = v.toObject();
            PaletteDef p;
            p.id = o.value(QStringLiteral("id")).toString();
            const auto read = [&](const char *k, std::array<float, 3> &to) {
                const QJsonArray a = o.value(QLatin1String(k)).toArray();
                for (int i = 0; i < 3; i++)
                    to[size_t(i)] = float(a.at(i).toDouble());
            };
            read("a", p.a);
            read("b", p.b);
            read("c", p.c);
            read("d", p.d);
            list.push_back(p);
        }
    }
    if (list.isEmpty())
        list.push_back({QStringLiteral("rainbow"), {0.5f, 0.5f, 0.5f}, {0.48f, 0.48f, 0.48f}, {1, 1, 1}, {0, 0.33f, 0.67f}});
    return list;
}

Palettes::Palettes(const QVector<PaletteDef> &list, unsigned seed) : list_(list), rand_(seed)
{
    from_ = to_ = int(rand_() % unsigned(list_.size()));
    blend();
}

void Palettes::blend()
{
    const double k = t_ * t_ * (3 - 2 * t_);
    const PaletteDef &f = list_[from_], &t = list_[to_];
    for (size_t i = 0; i < 3; i++) {
        out[0][i] = float(f.a[i] + (t.a[i] - f.a[i]) * k);
        out[1][i] = float(f.b[i] + (t.b[i] - f.b[i]) * k);
        out[2][i] = float(f.c[i] + (t.c[i] - f.c[i]) * k);
        out[3][i] = float(f.d[i] + (t.d[i] - f.d[i]) * k);
    }
}

void Palettes::step(double dt, int beatCount, bool calm)
{
    since_ += dt;
    const int everyBeats = calm ? 64 : 24;
    const double everySec = calm ? 60 : 25;
    if (t_ >= 1 && (beatCount - beatsAtChange_ >= everyBeats || since_ >= everySec)) {
        beatsAtChange_ = beatCount;
        since_ = 0;
        from_ = to_;
        for (int i = 0; i < 8 && to_ == from_; i++)
            to_ = int(rand_() % unsigned(list_.size()));
        t_ = 0;
    }
    if (t_ < 1) {
        t_ = std::min(1.0, t_ + dt / (calm ? 6.0 : 2.5));
        blend();
    }
}

bool systemWantsLessMotion()
{
#ifdef Q_OS_WIN
    BOOL animations = TRUE;
    if (SystemParametersInfoW(SPI_GETCLIENTAREAANIMATION, 0, &animations, 0))
        return !animations;
#endif
    return false;
}

} // namespace wv
