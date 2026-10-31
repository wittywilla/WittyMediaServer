// Checks for the desktop host, with no window. The renderer runs on a real graphics API,
// drawing into a texture, and pictures are read back, so this is the same code a window uses.
//
//   wvtests [--shots <folder>]        also writes a picture of every effect
#include "wvanalysis.h"
#include "wvengine.h"
#include "wvrenderer.h"
#include "wvtap.h"

#include <QCoreApplication>
#include <QElapsedTimer>
#include <QFile>
#include "wvwidget.h"

#include <QApplication>
#include <QImage>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QOffscreenSurface>
#include <QSettings>
#include <QTemporaryDir>
#include <QTest>
#include <cmath>
#include <rhi/qrhi.h>
#if defined(Q_OS_WIN)
#include <rhi/qrhi_platform.h>
#endif

using namespace wv;

static QString shotsDir;

namespace {

// tests/synth.mjs, for the same music on both sides.
void song(double t, double &l, double &r, double bpm = 120)
{
    const double two = 2 * 3.14159265358979323846;
    const double period = 60 / bpm, beat = t / period;
    const double since = std::fmod(std::max(0.0, t), period);
    const double kick = std::sin(two * (48 * since + (110.0 / 28) * (1 - std::exp(-since * 28)))) * std::exp(-since * 11) * 0.9;
    const int bar = int(std::fmod(std::floor(beat / 4), 4));
    static const double roots[] = {55, 65.4, 49, 73.4};
    const double root = roots[(bar + 4) % 4];
    const double half = std::fmod(beat * 2, 1.0);
    const double bass = std::sin(two * root * t) * 0.2 * (0.6 + 0.4 * std::exp(-half * 3));
    const double chord = (std::sin(two * root * 4 * t) + std::sin(two * root * 5 * t + 1) + std::sin(two * root * 6 * t + 2)) * 0.07;
    const double lead = std::sin(two * root * 12 * t + 3 * std::sin(two * 3 * t)) * 0.05 * (0.5 + 0.5 * std::sin(two * beat / 8));
    const double hat = (std::sin(t * 91345.7) * std::sin(t * 73123.3) + std::sin(t * 40123.9) * 0.5) * std::exp(-half * 18) * 0.13;
    double wash = 0, washR = 0;
    for (int i = 0; i < 18; i++) {
        const double hz = 90 * std::pow(1.3, i);
        const double swell = 0.5 + 0.5 * std::sin(t * (0.9 + i * 0.37) + i * 2.1);
        const double v = std::sin(two * hz * t + i) * swell * swell * 0.11 / (1 + i * 0.22);
        wash += v;
        washR += i % 2 ? v : -v * 0.6;
    }
    const auto hiss = [](double x) { const double v = std::sin(x * 12345.678) * 43758.5453; return (v - std::floor(v) - 0.5) * 0.05; };
    l = kick + bass + chord + lead + hat * 0.7 + wash + hiss(t);
    r = kick + bass + chord * 0.8 - lead + hat + washR + hiss(t + 7);
}

void fill(double t, int rate, std::vector<float> &left, std::vector<float> &right)
{
    const int n = int(left.size());
    for (int i = 0; i < n; i++) {
        double l, r;
        song(t - double(n - i) / rate, l, r);
        left[size_t(i)] = float(l);
        right[size_t(i)] = float(r);
    }
}

// A graphics API with no window.
struct Headless {
    std::unique_ptr<QRhi> rhi;
    std::unique_ptr<QOffscreenSurface> surface;
    Headless()
    {
#if defined(Q_OS_WIN)
        QRhiD3D11InitParams params;
        rhi.reset(QRhi::create(QRhi::D3D11, &params));
#endif
        if (!rhi) {
            QRhiNullInitParams null;
            rhi.reset(QRhi::create(QRhi::Null, &null));
        }
    }
    bool real() const { return rhi && rhi->backend() != QRhi::Null; }
};

// Runs effects into a texture and reads pictures back.
struct Bench {
    Headless h;
    Renderer r;
    std::unique_ptr<QRhiTexture> out;
    std::unique_ptr<QRhiTextureRenderTarget> rt;
    std::unique_ptr<QRhiRenderPassDescriptor> pass;
    Analyzer analyzer;
    Palettes palettes{builtinPalettes(), 1234};
    std::vector<float> left = std::vector<float>(kFftSize), right = std::vector<float>(kFftSize);
    double time = 0, flow = 0;
    int frameNo = 0;

    bool init(QSize size)
    {
        if (!h.real())
            return false;
        out.reset(h.rhi->newTexture(QRhiTexture::RGBA8, size, 1, QRhiTexture::RenderTarget | QRhiTexture::UsedAsTransferSource));
        out->create();
        rt.reset(h.rhi->newTextureRenderTarget({out.get()}));
        pass.reset(rt->newCompatibleRenderPassDescriptor());
        rt->setRenderPassDescriptor(pass.get());
        rt->create();
        if (!r.init(h.rhi.get(), pass.get()))
            return false;
        r.setSize(size);
        return true;
    }

    // One frame, optionally read back.
    QImage frame(const QString &a, const QString &b, double mix, bool calm, bool read, double fixedAudioAt = -1)
    {
        const double dt = 1.0 / 60;
        // Generating the test music is slow, so a speed test does it once.
        if (fixedAudioAt < 0 || frameNo == 0)
            fill(fixedAudioAt >= 0 ? fixedAudioAt : frameNo * dt, 44100, left, right);
        analyzer.process(left.data(), right.data(), 44100, dt);
        const AudioFrame &af = analyzer.frame();
        time += dt;
        flow += dt * (0.3 + 0.75 * af.level + 0.5 * af.beat);
        palettes.step(dt, af.beatCount, calm);
        frameNo++;
        QRhiCommandBuffer *cb = nullptr;
        h.rhi->beginOffscreenFrame(&cb);
        DrawInput f;
        f.audio = &af;
        f.time = time;
        f.flow = flow;
        f.dt = dt;
        f.calm = calm;
        f.palette = palettes.out;
        f.guard = calm ? kGuardCalm : kGuard;
        f.a = a;
        f.b = b;
        f.mix = mix;
        r.draw(cb, rt.get(), f);
        QRhiReadbackResult result;
        if (read) {
            QRhiResourceUpdateBatch *u = h.rhi->nextResourceUpdateBatch();
            QRhiReadbackDescription rb(out.get());
            u->readBackTexture(rb, &result);
            cb->resourceUpdate(u);
        }
        h.rhi->endOffscreenFrame();
        if (!read)
            return {};
        QImage img(reinterpret_cast<const uchar *>(result.data.constData()), result.pixelSize.width(), result.pixelSize.height(), QImage::Format_RGBA8888);
        return img.copy();
    }
};

double meanLuminance(const QImage &img)
{
    double sum = 0;
    const int w = img.width(), h = img.height();
    for (int y = 0; y < h; y++) {
        const QRgb *row = reinterpret_cast<const QRgb *>(img.constScanLine(y));
        for (int x = 0; x < w; x++) {
            const QColor c = QColor::fromRgba(row[x]);
            const auto lin = [](int v) { return std::pow(v / 255.0, 2.2); };
            sum += 0.2126 * lin(qRed(row[x])) + 0.7152 * lin(qGreen(row[x])) + 0.0722 * lin(qBlue(row[x]));
            Q_UNUSED(c);
        }
    }
    return sum / (w * h);
}

} // namespace

class Tests : public QObject {
    Q_OBJECT
private slots:
    void analysisMatchesTheWebVersion();
    void limitsMatchTheWebVersion();
    void everyEffectIsListedAndDifferent();
    void cycleAndBudget();
    void tapNeverBlocksAndReadsTheNewest();
    void tapIsFreeWhenOff();
    void settingsSurviveBadValues();
    void everyEffectDrawsAndLooksDifferent();
    void transitionsBlend();
    void strobeCannotGetThrough();
    void speed();
    void widgetDrawsWithNothingOnScreen();
};

void Tests::analysisMatchesTheWebVersion()
{
    QFile f(QStringLiteral(WV_SOURCE_DIR "/tests/fixtures/analysis-golden.json"));
    QVERIFY(f.open(QIODevice::ReadOnly));
    const QJsonArray golden = QJsonDocument::fromJson(f.readAll()).array();
    QVERIFY(golden.size() > 4);
    Analyzer a;
    std::vector<float> left(kFftSize), right(kFftSize);
    int at = 0;
    const auto near = [](double x, double y, double tol) { return std::abs(x - y) <= tol; };
    for (int frame = 0; frame < 240; frame++) {
        fill(frame / 60.0, 48000, left, right);
        a.process(left.data(), right.data(), 48000, 1.0 / 60);
        if (frame % 30 != 29)
            continue;
        const QJsonObject g = golden.at(at++).toObject();
        const AudioFrame &af = a.frame();
        const QByteArray where = "at frame " + QByteArray::number(frame);
        QVERIFY2(near(af.bass, g[QStringLiteral("bass")].toDouble(), 0.01), where);
        QVERIFY2(near(af.mid, g[QStringLiteral("mid")].toDouble(), 0.01), where);
        QVERIFY2(near(af.treble, g[QStringLiteral("treble")].toDouble(), 0.01), where);
        QVERIFY2(near(af.level, g[QStringLiteral("level")].toDouble(), 0.01), where);
        QVERIFY2(near(af.beat, g[QStringLiteral("beat")].toDouble(), 0.01), where);
        QCOMPARE(af.beatCount, g[QStringLiteral("beatCount")].toInt());
        QCOMPARE(af.historyRow, g[QStringLiteral("historyRow")].toInt());
        const auto check = [&](const char *key, int row, std::initializer_list<int> at2) {
            const QJsonArray want = g[QLatin1String(key)].toArray();
            int i = 0;
            for (int x : at2)
                QVERIFY2(near(af.texture[size_t(row) * kSpectrumBins + x], want.at(i++).toInt(), 2), key);
        };
        check("spectrum", 0, {8, 64, 128, 256, 384, 500});
        check("left", 1, {0, 100, 300, 500});
        check("right", 2, {0, 100, 300, 500});
        check("peaks", 3, {64, 256});
    }
}

void Tests::limitsMatchTheWebVersion()
{
    QFile f(QStringLiteral(WV_SOURCE_DIR "/safety.ts"));
    QVERIFY(f.open(QIODevice::ReadOnly));
    const QByteArray ts = f.readAll();
    QVERIFY(ts.contains("GUARD = { deadBand: 0.04, rise: 0.15 }"));
    QVERIFY(ts.contains("GUARD_CALM = { deadBand: 0.02, rise: 0.05 }"));
    QCOMPARE(kGuard.deadBand, 0.04f);
    QCOMPARE(kGuard.rise, 0.15f);
    QCOMPARE(kGuardCalm.deadBand, 0.02f);
    QCOMPARE(kGuardCalm.rise, 0.05f);
    QVERIFY(kGuard.deadBand + kGuard.rise / 3 < 0.1f);
}

void Tests::everyEffectIsListedAndDifferent()
{
    const auto &all = builtinEffects();
    QVERIFY(all.size() >= 12);
    QStringList ids;
    int gentle = 0;
    for (const EffectInfo &e : all) {
        QVERIFY(!e.id.isEmpty() && !e.name.isEmpty());
        QVERIFY(!ids.contains(e.id));
        ids << e.id;
        gentle += e.gentle;
    }
    QVERIFY(gentle >= 6);
    QVERIFY(builtinPalettes().size() >= 4);
}

void Tests::cycleAndBudget()
{
    Cycle c(5);
    const QStringList ids{QStringLiteral("a"), QStringLiteral("b"), QStringLiteral("c")};
    QString last;
    for (int i = 0; i < 30; i++) {
        const QString n = c.next(ids, true);
        QVERIFY(n != last);
        last = n;
    }
    const QString was = c.current;
    const QString n = c.next(ids, true);
    QCOMPARE(c.previous(ids, true), was);
    QVERIFY(n != was);

    Budget b(1, 0.25, 1);
    for (int i = 0; i < 400; i++)
        b.frame(1.0 / 15);
    QCOMPARE(b.scale, 0.25);
    QCOMPARE(b.frameGap, 1.0 / 30);
    Budget up(0.5, 0.25, 1);
    for (int i = 0; i < 3600; i++)
        up.frame(1.0 / 60);
    QCOMPARE(up.scale, 1.0);
}

void Tests::tapNeverBlocksAndReadsTheNewest()
{
    SampleTap tap;
    tap.setEnabled(true);
    std::vector<int16_t> pcm(4096 * 2);
    for (size_t i = 0; i < pcm.size(); i += 2) {
        pcm[i] = int16_t(i / 2);
        pcm[i + 1] = int16_t(-int(i / 2));
    }
    std::vector<float> l(kFftSize), r(kFftSize);
    QVERIFY(!tap.read(l.data(), r.data()));
    tap.write(pcm.data(), 4096);
    QVERIFY(tap.read(l.data(), r.data()));
    QCOMPARE(l[size_t(kFftSize - 1)], 4095.f / 32768.f);
    QCOMPARE(r[size_t(kFftSize - 1)], -4095.f / 32768.f);
    QVERIFY(!tap.read(l.data(), r.data())); // nothing new: this path isn't carrying sound
    // It waits as long as the sound takes to be heard.
    tap.setDelayFrames(1000);
    tap.write(pcm.data(), 1);
    QVERIFY(tap.read(l.data(), r.data()));
    // Far more than it holds, in one go, and it's still fine.
    std::vector<int16_t> lots(size_t(1 << 17) * 2, 7);
    tap.write(lots.data(), 1 << 17);
    tap.setDelayFrames(0);
    QVERIFY(tap.read(l.data(), r.data()));
    QCOMPARE(l[0], 7.f / 32768.f);

    // A writer as busy as an audio thread can be costs nanoseconds a frame.
    QElapsedTimer t;
    t.start();
    for (int i = 0; i < 20000; i++)
        tap.write(pcm.data(), 480);
    const double each = double(t.nsecsElapsed()) / 20000;
    QVERIFY2(each < 200000, qPrintable(QStringLiteral("%1 ns for 10 ms of sound").arg(each)));
    qInfo("the tap costs %.0f ns for each 10 ms of sound (%.4f%% of the audio thread's time)", each, each / 1e5);
}

void Tests::tapIsFreeWhenOff()
{
    SampleTap tap;
    std::vector<int16_t> pcm(4096 * 2, 5);
    tap.write(pcm.data(), 4096);
    std::vector<float> l(kFftSize), r(kFftSize);
    QVERIFY(!tap.read(l.data(), r.data()));
    QCOMPARE(l[0], 0.f);
}

void Tests::settingsSurviveBadValues()
{
    QTemporaryDir dir;
    QSettings s(dir.filePath(QStringLiteral("t.ini")), QSettings::IniFormat);
    s.beginGroup(QStringLiteral("visualizer"));
    s.setValue(QStringLiteral("cycleSec"), 7);
    s.setValue(QStringLiteral("intensity"), 90);
    s.setValue(QStringLiteral("sensitivity"), -4);
    s.setValue(QStringLiteral("calm"), 9);
    s.setValue(QStringLiteral("scale"), 0.2);
    s.setValue(QStringLiteral("effect"), QString(99, QLatin1Char('x')));
    s.endGroup();
    Settings v;
    v.load(s);
    QCOMPARE(v.notice, false);
    QCOMPARE(v.on, false);
    QCOMPARE(v.cycleSec, 30);
    QCOMPARE(v.intensity, 1.5);
    QCOMPARE(v.sensitivity, 0.25);
    QCOMPARE(v.calm, 1);
    QCOMPARE(v.scale, 0.0);
    QCOMPARE(v.effect.size(), 40);
    v.favourites = QStringList{QStringLiteral("a")};
    v.save(s);
    Settings back;
    back.load(s);
    QCOMPARE(back.favourites, v.favourites);
}

void Tests::everyEffectDrawsAndLooksDifferent()
{
    Bench b;
    if (!b.init(QSize(480, 270)))
        QSKIP("No real graphics API here");
    qInfo("drawing with %s on %s", b.h.rhi->backendName(), b.h.rhi->driverInfo().deviceName.constData());
    QVector<QImage> shots;
    QElapsedTimer t;
    for (const EffectInfo &e : builtinEffects()) {
        t.start();
        QVERIFY2(b.r.prepare(e), qPrintable(e.id + QStringLiteral(": ") + b.r.error()));
        qInfo("%s: ready in %lld ms", qPrintable(e.id), t.elapsed());
        QImage img;
        for (int i = 0; i < 300; i++)
            img = b.frame(e.id, {}, 0, false, i == 299);
        QVERIFY2(meanLuminance(img) > 0.002, qPrintable(e.id + QStringLiteral(" drew nothing")));
        shots.push_back(img);
        if (!shotsDir.isEmpty())
            img.save(shotsDir + QLatin1Char('/') + e.id + QStringLiteral(".png"));
        b.r.releaseTarget(e.id);
    }
    // No two effects give the same picture.
    for (int i = 0; i < shots.size(); i++) {
        for (int j = i + 1; j < shots.size(); j++) {
            const QImage x = shots[i].scaled(48, 27, Qt::IgnoreAspectRatio, Qt::SmoothTransformation), y = shots[j].scaled(48, 27, Qt::IgnoreAspectRatio, Qt::SmoothTransformation);
            double diff = 0;
            for (int py = 0; py < 27; py++)
                for (int px = 0; px < 48; px++)
                    diff += std::abs(qGray(x.pixel(px, py)) - qGray(y.pixel(px, py)));
            QVERIFY2(diff / (48 * 27) > 3, qPrintable(builtinEffects()[i].id + QStringLiteral(" and ") + builtinEffects()[j].id + QStringLiteral(" look alike")));
        }
    }
}

void Tests::transitionsBlend()
{
    Bench b;
    if (!b.init(QSize(160, 90)))
        QSKIP("No real graphics API here");
    const EffectInfo a = parseEffect([] { QFile f(QStringLiteral(":/wv/effects/hearth.glsl")); f.open(QIODevice::ReadOnly); return f.readAll(); }());
    const EffectInfo c = parseEffect([] { QFile f(QStringLiteral(":/wv/effects/molten.glsl")); f.open(QIODevice::ReadOnly); return f.readAll(); }());
    QVERIFY(b.r.prepare(a));
    QVERIFY(b.r.prepare(c));
    for (int i = 0; i < 120; i++)
        b.frame(a.id, c.id, double(i) / 119, false, false);
    QVERIFY(meanLuminance(b.frame(a.id, c.id, 1, false, true)) > 0.01);
}

void Tests::strobeCannotGetThrough()
{
    Bench b;
    if (!b.init(QSize(64, 36)))
        QSKIP("No real graphics API here");
    EffectInfo s;
    s.id = QStringLiteral("strobe");
    s.name = s.id;
    QVERIFY2(b.r.prepare(s), qPrintable(b.r.error()));
    for (bool calm : {false, true}) {
        QVector<double> vals;
        for (int i = 0; i < 400; i++)
            vals.push_back(meanLuminance(b.frame(s.id, {}, 0, calm, true)));
        double lo = 1, hi = 0;
        for (int i = 60; i < vals.size(); i++) {
            lo = std::min(lo, vals[i]);
            hi = std::max(hi, vals[i]);
        }
        qInfo("a black and white strobe comes out between %.3f and %.3f of full brightness (calm: %d)", lo, hi, calm);
        QVERIFY2(hi - lo < 0.1, "a swing this big counts as a flash");
    }
}

void Tests::speed()
{
    Bench b;
    if (!b.init(QSize(1280, 720)))
        QSKIP("No real graphics API here");
    QString line;
    for (const EffectInfo &e : builtinEffects()) {
        QVERIFY(b.r.prepare(e));
        for (int i = 0; i < 10; i++)
            b.frame(e.id, {}, 0, false, false, 1);
        QElapsedTimer t;
        t.start();
        const int n = 30;
        // Read back every frame, so this is the graphics card's time and not a queue's.
        for (int i = 0; i < n; i++)
            b.frame(e.id, {}, 0, false, true, 1);
        line += QStringLiteral("%1 %2, ").arg(e.id).arg(double(t.nsecsElapsed()) / n / 1e6, 0, 'f', 1);
        b.r.releaseTarget(e.id);
    }
    qInfo("1280x720 ms a frame: %s", qPrintable(line));
}

// Needs a platform with graphics, and never shows anything: run with -platform windows.
void Tests::widgetDrawsWithNothingOnScreen()
{
    if (QGuiApplication::platformName() == QStringLiteral("offscreen"))
        QSKIP("This platform has no graphics for a widget");
    QTemporaryDir dir;
    QSettings store(dir.filePath(QStringLiteral("t.ini")), QSettings::IniFormat);
    store.setValue(QStringLiteral("visualizer/notice"), true);
    store.setValue(QStringLiteral("visualizer/cycleSec"), 0);
    SampleTap tap;
    tap.setRate(44100);
    VisualizerWidget w(&tap, &store);
    w.setAttribute(Qt::WA_DontShowOnScreen);
    w.resize(640, 400);
    w.show();
    QTRY_VERIFY(tap.enabled());
    std::vector<int16_t> pcm(735 * 2);
    int at = 0;
    QImage img;
    QElapsedTimer t;
    t.start();
    int frames = 0;
    while (t.elapsed() < 6000) {
        for (int i = 0; i < 735; i++, at++) {
            double l, r;
            song(double(at) / 44100, l, r);
            pcm[size_t(i) * 2] = int16_t(std::clamp(l * 26000, -32000.0, 32000.0));
            pcm[size_t(i) * 2 + 1] = int16_t(std::clamp(r * 26000, -32000.0, 32000.0));
        }
        tap.write(pcm.data(), 735);
        img = w.view()->grabFramebuffer();
        QCoreApplication::processEvents();
        frames++;
    }
    QVERIFY2(!img.isNull(), "no picture came back");
    QVERIFY2(w.view()->problem().isEmpty(), qPrintable(w.view()->problem()));
    QVERIFY(!w.effectId().isEmpty());
    QVERIFY(w.view()->hearing());
    qInfo("%d frames in 6 seconds, showing %s, %d beats heard, drawn at %dx%d", frames, qPrintable(w.effectId()), w.view()->beats(), w.view()->drawSize().width(), w.view()->drawSize().height());
    QVERIFY2(w.view()->beats() >= 4, "it hears the beats");
    QVERIFY2(meanLuminance(img.convertToFormat(QImage::Format_RGBA8888)) > 0.001, "and draws something");
    if (!shotsDir.isEmpty())
        img.save(shotsDir + QStringLiteral("/widget.png"));
    const QString was = w.effectId();
    QVERIFY(w.handleKey(Qt::Key_Right));
    QVERIFY(w.effectId() != was);
    QVERIFY(w.handleKey(Qt::Key_Left));
    QVERIFY(!w.handleKey(Qt::Key_Q));
    w.hide();
    QVERIFY2(!tap.enabled(), "off is off");
}

int main(int argc, char **argv)
{
    QApplication app(argc, argv);
    QStringList args = app.arguments();
    const int at = args.indexOf(QStringLiteral("--shots"));
    if (at >= 0 && at + 1 < args.size()) {
        shotsDir = args[at + 1];
        args.remove(at, 2);
    }
    Tests t;
    return QTest::qExec(&t, args);
}

#include "tst_visualizer.moc"
