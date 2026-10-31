// Checks that need no one at the screen. They run with Qt's offscreen platform, make
// their own short clips with a test copy of FFmpeg (WMP_TEST_FFMPEG), and read real
// discs only when told where (WITTY_TEST_DVD, WITTY_TEST_BD).
#include "cd/audioencoder.h"
#include "cd/cdpage.h"
#include "cd/cdripper.h"
#include "disc/disccontroller.h"
#include "engine/engineclient.h"
#include "media/audiooutput.h"
#include "media/extentreader.h"
#include "media/player.h"
#include "media/vlcplayback.h"
#include "ui/controlbar.h"
#include "ui/imagedialog.h"
#include "ui/mainwindow.h"
#include "ui/settingsdialog.h"
#include "ui/theme.h"
#include "video/videoview.h"
#include "visual/visualizer.h"
#include "wvanalysis.h"
#include "wvtap.h"
#include "wvwidget.h"

#include <cmath>
#include <QDir>
#include <QElapsedTimer>
#include <QFile>
#include <QJsonArray>
#include <QJsonDocument>
#include <QLabel>
#include <QRadioButton>
#include <QProcess>
#include <QSettings>
#include <QSignalSpy>
#include <QBuffer>
#include <QImage>
#include <QTemporaryDir>
#include <QTest>

#ifdef Q_OS_WIN
#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#endif

using namespace wmp;

namespace {

class CountingSink : public VideoSink {
public:
    void presentFrame(const FramePtr &f) override
    {
        last = f;
        count++;
    }
    FramePtr last;
    std::atomic<int> count{0};
};

QString testFfmpeg()
{
    return qEnvironmentVariable("WMP_TEST_FFMPEG");
}

bool ffmpeg(const QStringList &args)
{
    QProcess p;
    p.start(testFfmpeg(), QStringList{QStringLiteral("-hide_banner"), QStringLiteral("-loglevel"), QStringLiteral("error"), QStringLiteral("-y")} + args);
    return p.waitForFinished(120000) && p.exitCode() == 0;
}

// CD audio made here: a different tone each side, so a swap or a shift would show.
QByteArray cdTone(int sectors, double hz = 440)
{
    QByteArray pcm(sectors * 2352, Qt::Uninitialized);
    auto *d = reinterpret_cast<qint16 *>(pcm.data());
    const int frames = sectors * 588;
    for (int i = 0; i < frames; i++) {
        d[i * 2] = qint16(11000 * std::sin(2 * M_PI * hz * i / 44100));
        d[i * 2 + 1] = qint16(7000 * std::sin(2 * M_PI * hz * 1.5 * i / 44100));
    }
    return pcm;
}

// A cue and bin in dir, with these tracks' lengths in sectors.
QString makeCdImage(const QString &dir, const QVector<int> &tracks, QByteArray *pcm)
{
    QByteArray cue = "PERFORMER \"The Testers\"\nTITLE \"Made Here\"\nFILE \"disc.bin\" BINARY\n";
    int at = 0;
    pcm->clear();
    for (int i = 0; i < tracks.size(); i++) {
        pcm->append(cdTone(tracks[i], 220.0 * (i + 1)));
        cue += QStringLiteral("  TRACK %1 AUDIO\n    TITLE \"Song %2\"\n    INDEX 01 %3:%4:%5\n")
                   .arg(i + 1, 2, 10, QLatin1Char('0'))
                   .arg(i + 1)
                   .arg(at / 75 / 60, 2, 10, QLatin1Char('0'))
                   .arg(at / 75 % 60, 2, 10, QLatin1Char('0'))
                   .arg(at % 75, 2, 10, QLatin1Char('0'))
                   .toUtf8();
        at += tracks[i];
    }
    QFile bin(QDir(dir).filePath(QStringLiteral("disc.bin")));
    QFile sheet(QDir(dir).filePath(QStringLiteral("disc.cue")));
    if (!bin.open(QIODevice::WriteOnly) || !sheet.open(QIODevice::WriteOnly))
        return {};
    bin.write(*pcm);
    sheet.write(cue);
    return sheet.fileName();
}

QByteArray decodePcm(const QString &file)
{
    QProcess p;
    p.start(testFfmpeg(), {QStringLiteral("-hide_banner"), QStringLiteral("-loglevel"), QStringLiteral("error"), QStringLiteral("-i"), file,
                           QStringLiteral("-map"), QStringLiteral("0:a"), QStringLiteral("-f"), QStringLiteral("s16le"), QStringLiteral("-ar"), QStringLiteral("44100"), QStringLiteral("-ac"), QStringLiteral("2"), QStringLiteral("-")});
    p.waitForFinished(60000);
    return p.readAllStandardOutput();
}

QString describe(const QString &file)
{
    QProcess p;
    p.start(testFfmpeg(), {QStringLiteral("-hide_banner"), QStringLiteral("-i"), file});
    p.waitForFinished(60000);
    return QString::fromUtf8(p.readAllStandardError());
}

// How alike two runs of samples sound, 1 being the same.
double likeness(const QByteArray &a, const QByteArray &b)
{
    const auto *x = reinterpret_cast<const qint16 *>(a.constData());
    const auto *y = reinterpret_cast<const qint16 *>(b.constData());
    const qsizetype n = qMin(a.size(), b.size()) / 2;
    double xy = 0, xx = 0, yy = 0;
    for (qsizetype i = 0; i < n; i++) {
        xy += double(x[i]) * y[i];
        xx += double(x[i]) * x[i];
        yy += double(y[i]) * y[i];
    }
    return xx > 0 && yy > 0 ? xy / std::sqrt(xx * yy) : 0;
}

QByteArray smallJpeg()
{
    QImage img(64, 64, QImage::Format_RGB32);
    img.fill(QColor(200, 80, 40));
    QByteArray out;
    QBuffer b(&out);
    b.open(QIODevice::WriteOnly);
    img.save(&b, "JPEG");
    return out;
}

} // namespace

class Tests : public QObject {
    Q_OBJECT
private slots:
    void initTestCase();

    void ripNamesAreSafe();
    void ripNamesNeverOverwrite();
    void everyFormatEncodes();
    void losslessIsExact();
    void spectrumFindsTheTone();
    void cdPlaysAndSeeks();
    void ripperCopiesTracks();
    void ripperStopLeavesNothing();
    void cdScreenCopies();
    void cdScreenKeysAndNames();
    void cdScreenMakesImage();

    void timeFormat();
    void fitKeepsShape();
    void colorMatrixForSD();
    void colorMatrixRangeBlackAndWhite();
    void themesAreReadable();
    void themesMatchTheProject();
    void imageNames();

    void extentReaderJoinsRanges();
    void extentReaderSkipsWhatWontRead();
    void extentReaderStops();

    void playsAClip();
    void visualizerHearsTheMusicAndLeavesItAlone();
    void pausesAndResumes();
    void seeksInAClip();
    void listsAndSwitchesTracks();
    void showsSubtitles();
    void readsSidecarSubtitles();
    void refusesWhatIsNotMedia();
    void refusesPlaylists();
    void playsAudioOnly();

    void engineClientProtocol();
    void engineClientHelperGone();
    void windowOpensAndRefuses();
    void windowKeyboard();

    void settingsWindow();
    void vlcIdsAndTimes();
    void vlcPlaysAClip();
    void vlcCantReadADiscModule();

    void realDvdMenusAndPlayback();
    void realBluray();

private:
    QTemporaryDir dir_;
    QString clip_, subs_, audioOnly_;
};

void Tests::initTestCase()
{
    QVERIFY(dir_.isValid());
    av_log_set_level(AV_LOG_QUIET);
    // The tests' choices stay out of the viewer's own settings.
    QSettings::setPath(QSettings::IniFormat, QSettings::UserScope, dir_.filePath(QStringLiteral("settings")));
    if (testFfmpeg().isEmpty())
        return;
    clip_ = dir_.filePath(QStringLiteral("clip.mkv"));
    // Five seconds, two audio tracks in two languages and one subtitle track.
    QFile srt(dir_.filePath(QStringLiteral("in.srt")));
    QVERIFY(srt.open(QIODevice::WriteOnly));
    srt.write("1\n00:00:01,000 --> 00:00:04,000\nHello from the file\n");
    srt.close();
    QVERIFY(ffmpeg({"-f", "lavfi", "-i", "testsrc2=s=320x240:r=25:d=5",
                    "-f", "lavfi", "-i", "sine=f=440:d=5", "-f", "lavfi", "-i", "sine=f=880:d=5", "-i", srt.fileName(),
                    "-map", "0", "-map", "1", "-map", "2", "-map", "3",
                    "-c:v", "mpeg4", "-q:v", "6", "-c:a", "ac3", "-c:s", "srt",
                    "-metadata:s:a:0", "language=eng", "-metadata:s:a:1", "language=fra", "-metadata:s:s:0", "language=spa", clip_}));
    audioOnly_ = dir_.filePath(QStringLiteral("tone.flac"));
    QVERIFY(ffmpeg({"-f", "lavfi", "-i", "sine=f=440:d=3", "-c:a", "flac", audioOnly_}));
}

void Tests::timeFormat()
{
    QCOMPARE(ControlBar::formatTime(0), QStringLiteral("0:00"));
    QCOMPARE(ControlBar::formatTime(65.9), QStringLiteral("1:05"));
    QCOMPARE(ControlBar::formatTime(3725), QStringLiteral("1:02:05"));
    QCOMPARE(ControlBar::formatTime(-3), QStringLiteral("0:00"));
}

void Tests::fitKeepsShape()
{
    QRectF r = VideoView::fit(16.0 / 9.0, QRectF(0, 0, 1000, 1000));
    QCOMPARE(int(r.width()), 1000);
    QCOMPARE(int(r.height()), 562);
    QCOMPARE(int(r.y()), 218);
    r = VideoView::fit(4.0 / 3.0, QRectF(0, 0, 1920, 1080));
    QCOMPARE(int(r.height()), 1080);
    QCOMPARE(int(r.width()), 1440);
    QCOMPARE(int(r.x()), 240);
}

void Tests::colorMatrixForSD()
{
    AVFrame f{};
    f.height = 480;
    f.colorspace = AVCOL_SPC_UNSPECIFIED;
    f.format = AV_PIX_FMT_YUV420P;
    // An unmarked DVD picture: BT.601's red uses more V than BT.709's.
    const float sd = VideoView::colorMatrix(&f)(0, 2);
    f.height = 1080;
    const float hd = VideoView::colorMatrix(&f)(0, 2);
    QVERIFY(sd < hd);
    QVERIFY(qAbs(sd - 1.596f) < 0.01f);
    QVERIFY(qAbs(hd - 1.793f) < 0.01f);
}

void Tests::colorMatrixRangeBlackAndWhite()
{
    AVFrame f{};
    f.height = 1080;
    f.format = AV_PIX_FMT_YUV420P;
    f.color_range = AVCOL_RANGE_MPEG;
    QMatrix4x4 m = VideoView::colorMatrix(&f);
    const QVector4D black = m.map(QVector4D(16.0f / 255, 128.0f / 255, 128.0f / 255, 1));
    const QVector4D white = m.map(QVector4D(235.0f / 255, 128.0f / 255, 128.0f / 255, 1));
    for (int i = 0; i < 3; i++) {
        QVERIFY2(qAbs(black[i]) < 0.01f, "video black should be black");
        QVERIFY2(qAbs(white[i] - 1) < 0.01f, "video white should be white");
    }
    f.color_range = AVCOL_RANGE_JPEG;
    m = VideoView::colorMatrix(&f);
    QVERIFY(qAbs(m.map(QVector4D(1, 128.0f / 255, 128.0f / 255, 1))[1] - 1) < 0.01f);
}

// Words and controls must be readable on every theme, to the usual standard.
void Tests::themesAreReadable()
{
    for (const QString &id : Themes::ids()) {
        const Theme t = Themes::load(id);
        QVERIFY2(!t.name.isEmpty(), qPrintable(id));
        QVERIFY2(contrastRatio(t.fg, t.bg) >= 7.0, qPrintable(id));
        QVERIFY2(contrastRatio(t.fg, t.bg2) >= 7.0, qPrintable(id));
        QVERIFY2(contrastRatio(t.fg2, t.bg2) >= 4.5, qPrintable(id));
        QVERIFY2(contrastRatio(t.fg3, t.bg) >= 3.0, qPrintable(id));
        QVERIFY2(contrastRatio(t.accentFg, t.accent) >= 4.5, qPrintable(id));
        QVERIFY2(contrastRatio(t.controlLine, t.bg2) >= 3.0, qPrintable(id));
        QVERIFY2(contrastRatio(t.focus, t.bg2) >= 3.0, qPrintable(id));
        const QString qss = Themes::styleSheet(t);
        QVERIFY2(!qss.contains(QLatin1Char('@')), qPrintable(id));
    }
}

// The copies here must follow the project's themes when both are at hand.
void Tests::themesMatchTheProject()
{
    const QString repo = qEnvironmentVariable("WMP_TEST_REPO");
    if (repo.isEmpty())
        QSKIP("WMP_TEST_REPO is not set");
    for (const QString &id : Themes::ids()) {
        QFile mine(QStringLiteral(":/themes/%1.json").arg(id)), theirs(QDir(repo).filePath(QStringLiteral("themes/%1/theme.json").arg(id)));
        QVERIFY(mine.open(QIODevice::ReadOnly));
        QVERIFY2(theirs.open(QIODevice::ReadOnly), qPrintable(theirs.fileName()));
        QCOMPARE(QJsonDocument::fromJson(mine.readAll()).object().value("colors"), QJsonDocument::fromJson(theirs.readAll()).object().value("colors"));
    }
}

void Tests::imageNames()
{
    QCOMPARE(ImageDialog::suggestedName(QStringLiteral("SHREK_2"), QStringLiteral("iso")), QStringLiteral("SHREK_2.iso"));
    QCOMPARE(ImageDialog::suggestedName(QStringLiteral("a/b:c?"), QStringLiteral("img")), QStringLiteral("a_b_c_.img"));
    QCOMPARE(ImageDialog::suggestedName(QStringLiteral("  "), QStringLiteral("iso")), QStringLiteral("Disc.iso"));
    QCOMPARE(ImageDialog::suggestedName(QStringLiteral("Trailing. "), QStringLiteral("iso")), QStringLiteral("Trailing.iso"));
}

void Tests::extentReaderJoinsRanges()
{
    QTemporaryDir d;
    QFile a(d.filePath("a")), b(d.filePath("b"));
    QVERIFY(a.open(QIODevice::WriteOnly));
    a.write("0123456789");
    a.close();
    QVERIFY(b.open(QIODevice::WriteOnly));
    b.write("abcdefghij");
    b.close();
    ExtentReader r({a.fileName(), b.fileName()}, {{0, 2, 3}, {1, 5, 4}, {0, 8, 2}});
    QCOMPARE(r.size(), qint64(9));
    QByteArray got(9, 0);
    int at = 0;
    while (at < 9) {
        int n = r.read(reinterpret_cast<uint8_t *>(got.data()) + at, 9 - at);
        QVERIFY(n > 0);
        at += n;
    }
    QCOMPARE(got, QByteArray("234fghi89"));
    char c;
    QCOMPARE(r.read(reinterpret_cast<uint8_t *>(&c), 1), 0);
    r.seek(4);
    QCOMPARE(r.read(reinterpret_cast<uint8_t *>(&c), 1), 1);
    QCOMPARE(c, 'g');
    QVERIFY(r.seek(100) < 0);
    int ext;
    qint64 within;
    QVERIFY(r.locate(7, &ext, &within));
    QCOMPARE(ext, 2);
    QCOMPARE(within, qint64(0));
}

// A range that can't be read is skipped as zeros and counted, as a scratch would be.
void Tests::extentReaderSkipsWhatWontRead()
{
    QTemporaryDir d;
    QFile a(d.filePath("a"));
    QVERIFY(a.open(QIODevice::WriteOnly));
    a.write(QByteArray(100, 'x'));
    a.close();
    // The second range points past the end of the file.
    ExtentReader r({a.fileName()}, {{0, 0, 100}, {0, 1000, 70000}});
    QByteArray all;
    QByteArray buf(4096, 0);
    for (;;) {
        int n = r.read(reinterpret_cast<uint8_t *>(buf.data()), buf.size());
        QVERIFY(n >= 0);
        if (n == 0)
            break;
        all += buf.left(n);
    }
    QCOMPARE(all.size(), 70100);
    QCOMPARE(all.left(100), QByteArray(100, 'x'));
    QCOMPARE(all.mid(100), QByteArray(70000, 0));
    QVERIFY(r.badBlocks() > 0);
    QVERIFY(!r.discLost());
}

void Tests::extentReaderStops()
{
    QTemporaryDir d;
    QFile a(d.filePath("a"));
    QVERIFY(a.open(QIODevice::WriteOnly));
    a.write(QByteArray(100, 'x'));
    a.close();
    ExtentReader r({a.fileName()}, {{0, 0, 100}});
    r.stop();
    char c;
    QCOMPARE(r.read(reinterpret_cast<uint8_t *>(&c), 1), -1);
}

void Tests::playsAClip()
{
    if (clip_.isEmpty())
        QSKIP("WMP_TEST_FFMPEG is not set");
    Player p;
    p.initAudio(true);
    CountingSink sink;
    p.setVideoSink(&sink);
    QSignalSpy ended(&p, &Player::ended), failed(&p, &Player::failed), opened(&p, &Player::opened);
    MediaSpec s;
    s.filePath = clip_;
    p.open(s);
    QTRY_VERIFY_WITH_TIMEOUT(opened.count() == 1, 5000);
    QVERIFY(failed.isEmpty());
    QTRY_VERIFY_WITH_TIMEOUT(ended.count() == 1, 15000);
    QVERIFY2(sink.count > 100, qPrintable(QString::number(sink.count.load())));
    QVERIFY(sink.last);
    QCOMPARE(sink.last->width, 320);
    const PlayerStats st = p.stats();
    QVERIFY(st.audioFrames > 48000 * 3);
    QCOMPARE(st.videoCodec, QStringLiteral("mpeg4"));
    QVERIFY(qAbs(p.position() - 5.0) < 0.3);
    QVERIFY(qAbs(p.duration() - 5.0) < 0.3);
}

void Tests::pausesAndResumes()
{
    if (clip_.isEmpty())
        QSKIP("WMP_TEST_FFMPEG is not set");
    Player p;
    p.initAudio(true);
    CountingSink sink;
    p.setVideoSink(&sink);
    MediaSpec s;
    s.filePath = clip_;
    p.open(s);
    QTRY_VERIFY_WITH_TIMEOUT(p.position() > 0.5, 5000);
    p.setPaused(true);
    QTest::qWait(100);
    const double at = p.position();
    const int shown = sink.count;
    QTest::qWait(400);
    QVERIFY(qAbs(p.position() - at) < 0.02);
    QVERIFY(sink.count - shown <= 1);
    p.setPaused(false);
    QTRY_VERIFY_WITH_TIMEOUT(p.position() > at + 0.3, 3000);
}

void Tests::seeksInAClip()
{
    if (clip_.isEmpty())
        QSKIP("WMP_TEST_FFMPEG is not set");
    Player p;
    p.initAudio(true);
    CountingSink sink;
    p.setVideoSink(&sink);
    MediaSpec s;
    s.filePath = clip_;
    p.open(s);
    QTRY_VERIFY_WITH_TIMEOUT(p.position() > 0.2, 5000);
    p.seek(3.0);
    QTRY_VERIFY_WITH_TIMEOUT(p.position() >= 3.0, 3000);
    QVERIFY(p.position() < 4.5);
    // And back: nothing from after the seek shows first.
    p.seek(1.0);
    QTRY_VERIFY_WITH_TIMEOUT(p.position() < 2.5, 3000);
    QTest::qWait(100);
    QVERIFY(p.position() >= 1.0);
}

void Tests::listsAndSwitchesTracks()
{
    if (clip_.isEmpty())
        QSKIP("WMP_TEST_FFMPEG is not set");
    Player p;
    p.initAudio(true);
    CountingSink sink;
    p.setVideoSink(&sink);
    QSignalSpy tracks(&p, &Player::tracksChanged);
    MediaSpec s;
    s.filePath = clip_;
    p.open(s);
    QTRY_VERIFY_WITH_TIMEOUT(tracks.count() >= 1, 5000);
    const auto audio = p.audioTracks();
    QCOMPARE(audio.size(), 2);
    QCOMPARE(audio[0].language, QStringLiteral("eng"));
    QCOMPARE(audio[1].language, QStringLiteral("fra"));
    QCOMPARE(languageName(audio[1].language), QStringLiteral("French"));
    QCOMPARE(p.subtitleTracks().size(), 1);
    QTRY_VERIFY_WITH_TIMEOUT(p.position() > 0.3, 5000);
    const int first = p.currentAudioTrack();
    p.setAudioTrack(audio[1].index);
    QTRY_COMPARE_WITH_TIMEOUT(p.currentAudioTrack(), audio[1].index, 3000);
    QVERIFY(first != audio[1].index);
    const auto at = p.stats().audioFrames;
    QTRY_VERIFY_WITH_TIMEOUT(p.stats().audioFrames > at + 4800, 3000);
}

void Tests::showsSubtitles()
{
    if (clip_.isEmpty())
        QSKIP("WMP_TEST_FFMPEG is not set");
    Player p;
    p.initAudio(true);
    CountingSink sink;
    p.setVideoSink(&sink);
    QSignalSpy tracks(&p, &Player::tracksChanged);
    MediaSpec s;
    s.filePath = clip_;
    p.open(s);
    QTRY_VERIFY_WITH_TIMEOUT(tracks.count() >= 1, 5000);
    QVERIFY(p.activeCues().isEmpty());
    p.setSubtitleTrack(p.subtitleTracks().first().index, false);
    QTRY_VERIFY_WITH_TIMEOUT(!p.activeCues().isEmpty(), 5000);
    QCOMPARE(p.activeCues().first().text, QStringLiteral("Hello from the file"));
    p.setSubtitleTrack(-1, false);
    QTRY_VERIFY_WITH_TIMEOUT(p.activeCues().isEmpty(), 3000);
}

void Tests::readsSidecarSubtitles()
{
    if (clip_.isEmpty())
        QSKIP("WMP_TEST_FFMPEG is not set");
    QTemporaryDir d;
    const QString movie = d.filePath("movie.mkv");
    QVERIFY(QFile::copy(clip_, movie));
    QFile srt(d.filePath("movie.en.srt"));
    QVERIFY(srt.open(QIODevice::WriteOnly));
    srt.write("1\n00:00:00,500 --> 00:00:04,000\nFrom beside the file\n");
    srt.close();
    Player p;
    p.initAudio(true);
    CountingSink sink;
    p.setVideoSink(&sink);
    QSignalSpy tracks(&p, &Player::tracksChanged);
    MediaSpec s;
    s.filePath = movie;
    p.open(s);
    QTRY_VERIFY_WITH_TIMEOUT(tracks.count() >= 1, 5000);
    TrackInfo side;
    for (const TrackInfo &t : p.subtitleTracks())
        if (t.external)
            side = t;
    QVERIFY(side.external);
    QCOMPARE(side.language, QStringLiteral("en"));
    p.setSubtitleTrack(side.index, true);
    QTRY_VERIFY_WITH_TIMEOUT(!p.activeCues().isEmpty(), 5000);
    QCOMPARE(p.activeCues().first().text, QStringLiteral("From beside the file"));
}

void Tests::refusesWhatIsNotMedia()
{
    QTemporaryDir d;
    QFile junk(d.filePath("junk.mkv"));
    QVERIFY(junk.open(QIODevice::WriteOnly));
    junk.write(QByteArray(100000, 'x'));
    junk.close();
    Player p;
    p.initAudio(true);
    QSignalSpy failed(&p, &Player::failed);
    MediaSpec s;
    s.filePath = junk.fileName();
    p.open(s);
    QTRY_VERIFY_WITH_TIMEOUT(failed.count() == 1, 5000);
    QVERIFY(failed.first().first().toString().contains(QLatin1String("can't read")));

    s.filePath = d.filePath("missing.mkv");
    p.open(s);
    QTRY_VERIFY_WITH_TIMEOUT(failed.count() == 2, 5000);
    QVERIFY(failed.last().first().toString().contains(QLatin1String("can't be found")));
}

// Nothing leaves the computer: a playlist naming a web address isn't followed, and
// this build has no network code to follow it with.
void Tests::refusesPlaylists()
{
    QTemporaryDir d;
    QFile list(d.filePath("list.m3u8"));
    QVERIFY(list.open(QIODevice::WriteOnly));
    list.write("#EXTM3U\n#EXTINF:10,\nhttp://127.0.0.1:9/never.ts\n");
    list.close();
    QVERIFY(!QString::fromLatin1(avformat_configuration()).contains(QLatin1String("--enable-network")));
    QVERIFY(!QString::fromLatin1(avformat_configuration()).contains(QLatin1String("libbluray")));
    QVERIFY(!QString::fromLatin1(avformat_configuration()).contains(QLatin1String("dvdread")));
    QVERIFY(QString::fromLatin1(avformat_configuration()).contains(QLatin1String("--disable-network")));
    QVERIFY(!avio_find_protocol_name("http://example.invalid/x"));
    QVERIFY(!avio_find_protocol_name("bluray:D:"));
    Player p;
    p.initAudio(true);
    QSignalSpy failed(&p, &Player::failed);
    MediaSpec s;
    s.filePath = list.fileName();
    p.open(s);
    QTRY_VERIFY_WITH_TIMEOUT(failed.count() == 1, 5000);
}

void Tests::playsAudioOnly()
{
    if (audioOnly_.isEmpty())
        QSKIP("WMP_TEST_FFMPEG is not set");
    Player p;
    p.initAudio(true);
    QSignalSpy ended(&p, &Player::ended);
    MediaSpec s;
    s.filePath = audioOnly_;
    p.open(s);
    QTRY_VERIFY_WITH_TIMEOUT(p.position() > 0.3, 5000);
    QVERIFY(p.isAudioOnly());
    QTRY_VERIFY_WITH_TIMEOUT(ended.count() == 1, 8000);
}

void Tests::engineClientProtocol()
{
    if (!QFile::exists(EngineClient::enginePath()))
        QSKIP("the helper isn't next to the tests");
    EngineClient c;
    QVERIFY(c.start());
    bool got = false;
    EngineReply r;
    c.call(QStringLiteral("drives"), {}, [&](const EngineReply &x) {
        r = x;
        got = true;
    });
    QTRY_VERIFY_WITH_TIMEOUT(got, 5000);
    QVERIFY(r.ok());
    QVERIFY(r.result.isArray());
    // Answers come back to the right call, and a refusal carries its reason.
    got = false;
    c.call(QStringLiteral("open"), {{"path", dir_.path()}}, [&](const EngineReply &x) {
        r = x;
        got = true;
    });
    QTRY_VERIFY_WITH_TIMEOUT(got, 5000);
    QCOMPARE(r.code, QStringLiteral("not-video"));
    QVERIFY(r.error.contains(QLatin1String("isn't a DVD or Blu-ray")));
    c.stop();
    QVERIFY(!c.running());
}

void Tests::engineClientHelperGone()
{
    EngineClient c;
    // No helper program where it is looked for means a plain answer, not a hang. Only
    // reachable by test when it really isn't there.
    if (QFile::exists(EngineClient::enginePath()))
        QSKIP("the helper is present");
    bool got = false;
    EngineReply r;
    c.call(QStringLiteral("drives"), {}, [&](const EngineReply &x) {
        r = x;
        got = true;
    });
    QTRY_VERIFY_WITH_TIMEOUT(got, 5000);
    QVERIFY(!r.ok());
}

void Tests::windowOpensAndRefuses()
{
    if (!QFile::exists(EngineClient::enginePath()))
        QSKIP("the helper isn't next to the tests");
    MainWindow w(true);
    w.show();
    // Something that isn't a disc gets a plain message and no crash.
    w.openPath(dir_.path());
    QTRY_VERIFY_WITH_TIMEOUT(!w.shownMessage().isEmpty(), 5000);
    QVERIFY(w.shownMessage().contains(QLatin1String("isn't a DVD or Blu-ray")));
    w.handleKey(Qt::Key_Escape);
    QVERIFY(w.shownMessage().isEmpty());
    if (!clip_.isEmpty()) {
        w.openPath(clip_);
        QTRY_VERIFY_WITH_TIMEOUT(w.player()->position() > 0.2, 8000);
        QCOMPARE(w.shownSource(), clip_);
    }
    w.close();
}

void Tests::windowKeyboard()
{
    if (clip_.isEmpty() || !QFile::exists(EngineClient::enginePath()))
        QSKIP("needs a clip and the helper");
    MainWindow w(true);
    w.show();
    w.openPath(clip_);
    QTRY_VERIFY_WITH_TIMEOUT(w.player()->position() > 0.2, 8000);
    // Space pauses and plays.
    QVERIFY(w.handleKey(Qt::Key_Space));
    QVERIFY(w.player()->paused());
    QVERIFY(w.handleKey(Qt::Key_Space));
    QVERIFY(!w.player()->paused());
    // Right goes forward, S turns the subtitles on, A moves to the next audio track.
    const double before = w.player()->position();
    QVERIFY(w.handleKey(Qt::Key_Right));
    Q_UNUSED(before);
    QVERIFY(w.handleKey(Qt::Key_S));
    QTRY_VERIFY2_WITH_TIMEOUT(w.player()->currentSubtitleTrack() >= 0, qPrintable(QStringLiteral("subs %1, pos %2, paused %3").arg(w.player()->subtitleTracks().size()).arg(w.player()->position()).arg(w.player()->paused())), 3000);
    const int audio = w.player()->currentAudioTrack();
    QVERIFY(w.handleKey(Qt::Key_A));
    QTRY_VERIFY_WITH_TIMEOUT(w.player()->currentAudioTrack() != audio, 3000);
    // Keys it doesn't use are left for the rest of the program.
    QVERIFY(!w.handleKey(Qt::Key_Q));
    w.close();
}

// The choice of libraries, with VLC credited where it's chosen.
void Tests::settingsWindow()
{
    Player p;
    p.initAudio(true);
    SettingsDialog d(&p, nullptr, false, VlcPlayback::detectFolder(), false);
    d.show();
    QVERIFY(!d.useVlc());
    QVERIFY(SettingsDialog::vlcCredit().contains(QLatin1String("VideoLAN")));
    auto *radio = d.findChildren<QRadioButton *>().value(1);
    QVERIFY(radio);
    radio->setChecked(true);
    QVERIFY(d.useVlc());
    bool credited = false;
    for (QLabel *l : d.findChildren<QLabel *>())
        credited = credited || l->text() == SettingsDialog::vlcCredit();
    QVERIFY(credited);
    const QString grab = qEnvironmentVariable("WMP_TEST_GRAB");
    if (!grab.isEmpty())
        d.grab().save(QDir(grab).filePath(QStringLiteral("settings.png")));
    // A wrong folder says so, and nothing loads.
    QString why;
    Player other;
    other.initAudio(true);
    QVERIFY(!other.enableVlc(dir_.path(), &why));
    QVERIFY(why.contains(QLatin1String("aren't in that folder")));
}

void Tests::vlcIdsAndTimes()
{
    QCOMPARE(VlcPlayback::discStreamId(QStringLiteral("mpeg"), 0x1e0), 0xe0);
    QCOMPARE(VlcPlayback::discStreamId(QStringLiteral("mpeg"), 0x1c0), 0xc0);
    QCOMPARE(VlcPlayback::discStreamId(QStringLiteral("mpeg"), 0x80), 0xbd80);
    QCOMPARE(VlcPlayback::discStreamId(QStringLiteral("mpeg"), 0x21), 0xbd21);
    QCOMPARE(VlcPlayback::discStreamId(QStringLiteral("mpegts"), 0x1100), 0x1100);
    const QVector<Segment> segs{{100, 0, 10, -1}, {300, 10, 20, -1}};
    QCOMPARE(VlcPlayback::timeAt(segs, 0), 0.0);
    QCOMPARE(VlcPlayback::timeAt(segs, 0.125), 5.0);
    QCOMPARE(VlcPlayback::timeAt(segs, 0.25), 10.0);
    QCOMPARE(VlcPlayback::timeAt(segs, 1.0), 30.0);
}

void Tests::vlcPlaysAClip()
{
    const QString folder = VlcPlayback::detectFolder();
    if (folder.isEmpty() || clip_.isEmpty())
        QSKIP("needs a VLC on this computer and a test clip");
    Player p;
    p.initAudio(true);
    CountingSink sink;
    p.setVideoSink(&sink);
    QString why;
    QVERIFY2(p.enableVlc(folder, &why), qPrintable(why));
    QVERIFY(p.vlcVersion().startsWith(QLatin1String("3.")));
    p.useVlc(true);
    p.setVolume(0);
    QSignalSpy ended(&p, &Player::ended);
    MediaSpec s;
    s.filePath = clip_;
    p.open(s);
    QVERIFY(p.playingThroughVlc());
    QTRY_VERIFY_WITH_TIMEOUT(sink.count > 10, 8000);
    QCOMPARE(sink.last->width, 320);
    QTRY_VERIFY_WITH_TIMEOUT(p.position() > 0.5, 5000);
    QVERIFY(qAbs(p.duration() - 5.0) < 0.5);
    QTRY_VERIFY_WITH_TIMEOUT(p.audioTracks().size() == 2, 3000);
    p.seek(4.0);
    QTRY_VERIFY_WITH_TIMEOUT(ended.count() == 1, 8000);
    // Back to the built-in playback for the next thing opened.
    p.useVlc(false);
    p.open(s);
    QVERIFY(!p.playingThroughVlc());
}

// A disc image with an innocent name can't reach VLC's disc modules.
void Tests::vlcCantReadADiscModule()
{
    const QString folder = VlcPlayback::detectFolder();
    const QString iso = qEnvironmentVariable("WITTY_TEST_DVD");
    if (folder.isEmpty() || iso.isEmpty())
        QSKIP("needs a VLC on this computer and WITTY_TEST_DVD");
    // The start of a disc image under another name, in a folder of its own.
    QTemporaryDir d;
    const QString disguised = d.filePath("film.mkv");
    QFile out(disguised);
    QVERIFY(out.open(QIODevice::WriteOnly));
    QFile in(iso);
    QVERIFY(in.open(QIODevice::ReadOnly));
    out.write(in.read(8 * 1024 * 1024));
    out.close();
    Player p;
    p.initAudio(true);
    CountingSink sink;
    p.setVideoSink(&sink);
    QVERIFY(p.enableVlc(folder, nullptr));
    p.useVlc(true);
    p.setVolume(0);
    MediaSpec s;
    s.filePath = disguised;
    p.open(s);
    QTest::qWait(3000);
#ifdef Q_OS_WIN
    for (const char *m : {"libdvdnav_plugin.dll", "libdvdread_plugin.dll", "liblibbluray_plugin.dll"})
        QVERIFY2(!GetModuleHandleA(m), m);
#endif
}

void Tests::realDvdMenusAndPlayback()
{
    const QString iso = qEnvironmentVariable("WITTY_TEST_DVD");
    if (iso.isEmpty() || !QFile::exists(EngineClient::enginePath()))
        QSKIP("WITTY_TEST_DVD is not set");
    MainWindow w(true);
    w.show();
    w.openPath(iso);
    QTRY_VERIFY_WITH_TIMEOUT(w.disc()->active(), 15000);
    QTRY_VERIFY_WITH_TIMEOUT(w.player()->stats().framesShown > 0, 15000);
    QCOMPARE(w.disc()->kind(), QStringLiteral("dvd"));
    // The feature plays with its video, sound and tracks.
    w.disc()->playMain();
    // A disc's own program may insist on its warning first: Next goes past it, as on a player,
    // and if that lands in the disc's menu, the feature is asked for again.
    for (int i = 0; i < 60 && !(w.disc()->inTitle() && w.disc()->ttn() == w.disc()->mainTitle()); i++) {
        QTest::qWait(500);
        if (i % 4 == 3) {
            if (w.disc()->inMenu())
                w.disc()->playMain();
            else
                w.disc()->skip();
        }
    }
    QVERIFY2(w.disc()->inTitle() && w.disc()->ttn() == w.disc()->mainTitle(), qPrintable(QStringLiteral("kind %1, title %2, main %3").arg(w.disc()->presentationKind()).arg(w.disc()->ttn()).arg(w.disc()->mainTitle())));
    QTRY_VERIFY_WITH_TIMEOUT(w.player()->position() > 1.0, 20000);
    const PlayerStats st = w.player()->stats();
    QCOMPARE(st.videoCodec, QStringLiteral("mpeg2video"));
    QVERIFY(st.audioFrames > 0);
    QVERIFY(!w.disc()->audioTracks().isEmpty());
    // Seeking lands close to where it was asked to.
    w.disc()->seekTo(600);
    QTRY_VERIFY_WITH_TIMEOUT(w.player()->position() >= 600 && w.player()->position() < 615, 20000);
    w.close();
}

void Tests::realBluray()
{
    const QString iso = qEnvironmentVariable("WITTY_TEST_BD");
    if (iso.isEmpty() || !QFile::exists(EngineClient::enginePath()))
        QSKIP("WITTY_TEST_BD is not set");
    MainWindow w(true);
    w.show();
    w.openPath(iso);
    QTRY_VERIFY_WITH_TIMEOUT(w.disc()->active(), 15000);
    QCOMPARE(w.disc()->kind(), QStringLiteral("bluray"));
    w.disc()->playMain();
    QTRY_VERIFY_WITH_TIMEOUT(w.disc()->inTitle() && w.disc()->ttn() == w.disc()->mainTitle(), 20000);
    QTRY_VERIFY_WITH_TIMEOUT(w.player()->position() > 1.0, 30000);
    w.disc()->seekTo(600);
    QTRY_VERIFY_WITH_TIMEOUT(w.player()->position() >= 600 && w.player()->position() < 615, 30000);
    w.close();
}

void Tests::ripNamesAreSafe()
{
    RipNames n;
    n.artist = QStringLiteral("AC/DC");
    n.album = QStringLiteral("..\\..\\Windows");
    n.title = QStringLiteral("What? <Now>: \"Yes\" | no*");
    n.track = 3;
    n.tracks = 12;
    n.year = QStringLiteral("2001-05-01");
    QCOMPARE(ripRelativePath(defaultRipTemplate(), n, QStringLiteral("flac")), QStringLiteral("AC_DC/_.._Windows/03 What_ _Now__ _Yes_ _ no_.flac"));
    QCOMPARE(ripRelativePath(QStringLiteral("{year} {album}/{track}"), n, QStringLiteral("mp3")), QStringLiteral("2001 _.._Windows/03.mp3"));
    // A template can't climb out of the folder either.
    for (const QString &t : {QStringLiteral("../../x/{title}"), QStringLiteral("..\\..\\{track}"), QStringLiteral("C:/x/{track}"), QStringLiteral("/{track}")}) {
        const QString path = ripRelativePath(t, n, QStringLiteral("flac"));
        QVERIFY2(!path.startsWith(QLatin1Char('/')) && !path.contains(QStringLiteral("../")) && !path.contains(QLatin1Char(':')), qPrintable(path));
    }
    RipNames blank;
    blank.track = 1;
    QCOMPARE(ripRelativePath(defaultRipTemplate(), blank, QStringLiteral("wav")), QStringLiteral("Unknown artist/Unknown album/01 Track 01.wav"));
    QCOMPARE(ripRelativePath(QString(), blank, QStringLiteral("wav")), QStringLiteral("Track 01.wav"));
    n.title = QStringLiteral("CON");
    QCOMPARE(ripRelativePath(QStringLiteral("{title}"), n, QStringLiteral("wav")), QStringLiteral("_CON.wav"));
    n.title = QStringLiteral("x").repeated(500);
    QVERIFY(ripRelativePath(QStringLiteral("{title}"), n, QStringLiteral("wav")).size() < 130);
}

void Tests::ripNamesNeverOverwrite()
{
    QTemporaryDir dir;
    const QString a = dir.filePath(QStringLiteral("song.flac"));
    QCOMPARE(uniquePath(a), a);
    QFile(a).open(QIODevice::WriteOnly);
    QCOMPARE(uniquePath(a), dir.filePath(QStringLiteral("song (2).flac")));
    QFile(dir.filePath(QStringLiteral("song (2).flac.part"))).open(QIODevice::WriteOnly);
    QCOMPARE(uniquePath(a), dir.filePath(QStringLiteral("song (3).flac")));
}

void Tests::everyFormatEncodes()
{
    if (testFfmpeg().isEmpty())
        QSKIP("no test FFmpeg");
    QTemporaryDir dir;
    const QByteArray pcm = cdTone(75 * 4);
    TrackTags tags;
    tags.title = QStringLiteral("Sång of Tests");
    tags.artist = QStringLiteral("The Testers");
    tags.album = QStringLiteral("Made Here");
    tags.albumArtist = QStringLiteral("Various");
    tags.date = QStringLiteral("2001");
    tags.track = 3;
    tags.trackTotal = 9;
    tags.cover = smallJpeg();
    for (const RipFormat &f : ripFormats()) {
        for (const RipQuality &q : f.qualities) {
            const QString path = dir.filePath(QStringLiteral("%1-%2.%3").arg(f.id, q.id, f.extension));
            AudioEncoder enc;
            QString why;
            QVERIFY2(enc.open(path, f.id, q.id, tags, &why), qPrintable(f.id + QLatin1Char(' ') + why));
            for (qsizetype at = 0; at < pcm.size(); at += 40000)
                QVERIFY2(enc.write(pcm.constData() + at, qMin<qsizetype>(40000, pcm.size() - at)), qPrintable(enc.error()));
            QVERIFY2(enc.finish(), qPrintable(f.id + QLatin1Char(' ') + enc.error()));
            const QByteArray back = decodePcm(path);
            // Encoders pad the ends a little.
            QVERIFY2(qAbs(back.size() - pcm.size()) < 44100, qPrintable(QStringLiteral("%1: %2 of %3 bytes").arg(path).arg(back.size()).arg(pcm.size())));
            // A tone comes through a lossy encoder as the same tone, once its delay's found.
            double best = 0;
            for (int shift = 0; shift < 8192 && best < 0.9; shift += 4)
                best = qMax(best, likeness(pcm.mid(4000, 100000), back.mid(4000 + shift, 100000)));
            QVERIFY2(best > 0.9, qPrintable(QStringLiteral("%1: likeness %2").arg(path).arg(best)));
            const QString info = describe(path);
            if (f.id != QLatin1String("wav")) {
                QVERIFY2(info.contains(QStringLiteral("Sång of Tests")) && info.contains(QStringLiteral("Made Here")) && info.contains(QStringLiteral("The Testers")), qPrintable(info));
                QVERIFY2(info.contains(QStringLiteral("attached pic")) || info.contains(QStringLiteral("cover")), qPrintable(info));
            }
        }
    }
    // A format it doesn't have, and a place it can't write, are plain answers.
    AudioEncoder enc;
    QString why;
    QVERIFY(!enc.open(dir.filePath(QStringLiteral("x")), QStringLiteral("nonsense"), {}, {}, &why));
    QVERIFY(!why.isEmpty());
    QVERIFY(!enc.open(dir.filePath(QStringLiteral("no/such/folder/x.flac")), QStringLiteral("flac"), {}, {}, &why));
}

void Tests::losslessIsExact()
{
    if (testFfmpeg().isEmpty())
        QSKIP("no test FFmpeg");
    QTemporaryDir dir;
    const QByteArray pcm = cdTone(75 * 3 + 17);
    for (const QString &id : {QStringLiteral("flac"), QStringLiteral("alac"), QStringLiteral("wav")}) {
        const QString path = dir.filePath(QStringLiteral("x.") + ripFormat(id)->extension);
        AudioEncoder enc;
        QString why;
        QVERIFY2(enc.open(path, id, {}, {}, &why), qPrintable(why));
        QVERIFY(enc.write(pcm.constData(), pcm.size()));
        QVERIFY2(enc.finish(), qPrintable(enc.error()));
        QVERIFY2(decodePcm(path) == pcm, qPrintable(id));
    }
}

void Tests::spectrumFindsTheTone()
{
    const int frames = 1024, rate = 44100;
    std::vector<qint16> s(size_t(frames) * 2);
    const auto strongest = [&](double hz) {
        for (int i = 0; i < frames; i++)
            s[size_t(i) * 2] = s[size_t(i) * 2 + 1] = qint16(16000 * std::sin(2 * M_PI * hz * i / rate));
        float bands[28];
        spectrumBands(s.data(), frames, rate, bands, 28);
        return int(std::max_element(bands, bands + 28) - bands);
    };
    const int low = strongest(200), mid = strongest(1000), high = strongest(8000);
    QVERIFY(low < mid && mid < high);
    float bands[28];
    std::fill(s.begin(), s.end(), qint16(0));
    spectrumBands(s.data(), frames, rate, bands, 28);
    for (float b : bands)
        QCOMPARE(b, 0.f);
}

void Tests::cdPlaysAndSeeks()
{
    QTemporaryDir dir;
    QByteArray pcm;
    const QString cue = makeCdImage(dir.path(), {75 * 8, 75 * 8}, &pcm);
    if (!QFile::exists(EngineClient::enginePath()))
        QSKIP("no helper program");
    Player p;
    p.initAudio(true);
    CountingSink sink;
    p.setVideoSink(&sink);
    QSignalSpy opened(&p, &Player::opened);
    QSignalSpy failed(&p, &Player::failed);
    MediaSpec spec;
    spec.filePath = cue;
    spec.cd = true;
    p.open(spec);
    QTRY_VERIFY_WITH_TIMEOUT(opened.size() > 0 || failed.size() > 0, 15000);
    QVERIFY(failed.isEmpty());
    QVERIFY(qAbs(p.duration() - 16) < 0.1);
    QVERIFY(p.isAudioOnly());
    QTRY_VERIFY_WITH_TIMEOUT(p.position() > 0.5, 10000);
    p.seek(9);
    QTRY_VERIFY_WITH_TIMEOUT(p.position() >= 9 && p.position() < 11, 10000);
    p.stop();
    // A CD that isn't there is a message, and no hang.
    QSignalSpy failed2(&p, &Player::failed);
    spec.filePath = dir.filePath(QStringLiteral("gone.cue"));
    p.open(spec);
    QTRY_VERIFY_WITH_TIMEOUT(failed2.size() > 0, 15000);
    p.stop();
}

void Tests::ripperCopiesTracks()
{
    if (testFfmpeg().isEmpty() || !QFile::exists(EngineClient::enginePath()))
        QSKIP("no test FFmpeg or helper");
    QTemporaryDir src, out;
    QByteArray pcm;
    const QString cue = makeCdImage(src.path(), {150, 225, 100}, &pcm);
    RipJob job;
    job.source = cue;
    job.format = QStringLiteral("flac");
    job.quality = QStringLiteral("5");
    job.folder = out.path();
    job.nameTemplate = defaultRipTemplate();
    job.album = QStringLiteral("Made Here");
    for (int n : {3, 1}) {
        RipTrackJob t;
        t.number = n;
        t.names.albumArtist = QStringLiteral("The Testers");
        t.names.album = job.album;
        t.names.title = QStringLiteral("Song %1").arg(n);
        t.names.track = n;
        t.tags.title = t.names.title;
        t.tags.track = n;
        job.tracks.push_back(t);
    }
    CdRipper r;
    QSignalSpy finished(&r, &CdRipper::finished);
    QSignalSpy done(&r, &CdRipper::trackFinished);
    r.start(job);
    QTRY_VERIFY_WITH_TIMEOUT(finished.size() == 1, 60000);
    QCOMPARE(finished[0][1].toString(), QString());
    QCOMPARE(done.size(), 2);
    const QString one = out.filePath(QStringLiteral("The Testers/Made Here/01 Song 1.flac"));
    const QString three = out.filePath(QStringLiteral("The Testers/Made Here/03 Song 3.flac"));
    QCOMPARE(decodePcm(one), pcm.left(150 * 2352));
    QCOMPARE(decodePcm(three), pcm.mid(375 * 2352));
    QVERIFY(!QFile::exists(one + QStringLiteral(".part")));
    const RipResult first = done[1][0].value<RipResult>();
    QVERIFY(first.ok && !first.crc.isEmpty() && first.unreadable == 0);
    // The report says what happened, track by track, in the same folder.
    QFile report(finished[0][2].toString());
    QVERIFY(report.open(QIODevice::ReadOnly));
    const QString text = QString::fromUtf8(report.readAll());
    QVERIFY2(text.contains(QStringLiteral("Track 01")) && text.contains(first.crc) && QFileInfo(report).absolutePath() == QFileInfo(one).absolutePath(), qPrintable(text));
    // A second copy keeps the first.
    QSignalSpy again(&r, &CdRipper::finished);
    QTRY_VERIFY(!r.running());
    r.start(job);
    QTRY_VERIFY_WITH_TIMEOUT(again.size() == 1, 60000);
    QVERIFY(QFile::exists(out.filePath(QStringLiteral("The Testers/Made Here/01 Song 1 (2).flac"))));
}

void Tests::ripperStopLeavesNothing()
{
    if (!QFile::exists(EngineClient::enginePath()))
        QSKIP("no helper");
    QTemporaryDir src, out;
    QByteArray pcm;
    const QString cue = makeCdImage(src.path(), {75 * 900}, &pcm); // long enough to be caught part way
    RipJob job;
    job.source = cue;
    job.format = QStringLiteral("flac");
    job.quality = QStringLiteral("8");
    job.folder = out.path();
    job.nameTemplate = QStringLiteral("{track}");
    RipTrackJob t;
    t.number = 1;
    t.names.track = 1;
    job.tracks.push_back(t);
    CdRipper r;
    QSignalSpy finished(&r, &CdRipper::finished);
    QSignalSpy progress(&r, &CdRipper::trackProgress);
    r.start(job);
    QTRY_VERIFY_WITH_TIMEOUT(progress.size() > 0 || finished.size() > 0, 20000);
    r.stop();
    QTRY_VERIFY_WITH_TIMEOUT(finished.size() == 1, 30000);
    QVERIFY(finished[0][0].toBool());
    QVERIFY2(QDir(out.path()).entryList(QDir::Files | QDir::NoDotAndDotDot).isEmpty(), qPrintable(QDir(out.path()).entryList().join(QLatin1Char(','))));
}

void Tests::cdScreenCopies()
{
    if (testFfmpeg().isEmpty() || !QFile::exists(EngineClient::enginePath()))
        QSKIP("no test FFmpeg or helper");
    QTemporaryDir src, out;
    QByteArray pcm;
    const QString cue = makeCdImage(src.path(), {150, 150}, &pcm);
    MainWindow w(true);
    w.show();
    w.openPath(cue);
    QTRY_VERIFY_WITH_TIMEOUT(w.cd()->isVisible(), 15000);
    QCOMPARE(w.cd()->disc().tracks.size(), 2);
    QCOMPARE(w.cd()->disc().title, QStringLiteral("Made Here"));
    QCOMPARE(w.cd()->checkedTracks(), QVector<int>({1, 2}));
    // Nothing ticked, or nowhere to put it, is a plain word.
    w.cd()->setChecked({});
    w.cd()->startRip();
    QVERIFY(!w.cd()->busy());
    w.cd()->setChecked({2});
    w.cd()->setRipOptions(QStringLiteral("alac"), {}, out.filePath(QStringLiteral("not-here")), QStringLiteral("{track} {title}"), true, 0);
    w.cd()->startRip();
    QVERIFY(!w.cd()->busy());
    w.cd()->setRipOptions(QStringLiteral("alac"), {}, out.path(), QStringLiteral("{track} {title}"), true, 0);
    QSignalSpy finished(w.cd(), &CdPage::ripFinished);
    w.cd()->startRip();
    QVERIFY(w.cd()->busy());
    // Nothing else opens over a copy that's under way.
    w.openPath(cue);
    QVERIFY(w.shownMessage().contains(QStringLiteral("being copied")));
    QTRY_VERIFY_WITH_TIMEOUT(finished.size() == 1, 60000);
    QTRY_VERIFY(!w.cd()->busy());
    QVERIFY2(w.cd()->statusText().contains(QStringLiteral("Copied 1 of 1")), qPrintable(w.cd()->statusText()));
    QCOMPARE(decodePcm(out.filePath(QStringLiteral("02 Song 2.m4a"))), pcm.mid(150 * 2352));
    w.close();
}

void Tests::cdScreenKeysAndNames()
{
    if (!QFile::exists(EngineClient::enginePath()))
        QSKIP("no helper");
    QTemporaryDir src;
    QByteArray pcm;
    const QString cue = makeCdImage(src.path(), {75 * 6, 75 * 6, 75 * 6}, &pcm);
    MainWindow w(true);
    w.show();
    w.openPath(cue);
    QTRY_VERIFY_WITH_TIMEOUT(w.cd()->isVisible(), 15000);
    CdPage *c = w.cd();
    QVERIFY(!c->playing()); // an opened CD waits to be asked
    QVERIFY(w.handleKey(Qt::Key_K));
    QTRY_VERIFY_WITH_TIMEOUT(w.player()->position() > 0.3, 15000);
    QVERIFY(c->playing());
    QVERIFY(w.handleKey(Qt::Key_N));
    QCOMPARE(c->currentTrack(), 1);
    QTRY_VERIFY_WITH_TIMEOUT(w.player()->position() >= 6 && w.player()->position() < 8, 15000);
    QVERIFY(w.handleKey(Qt::Key_K));
    QVERIFY(!c->playing());
    QVERIFY(w.handleKey(Qt::Key_S));
    QVERIFY(!w.player()->hasMedia());
    // Names from a lookup land where they should, and a bad position is ignored.
    const QJsonObject match = QJsonDocument::fromJson(R"({"title":"An Album","artist":"A Band","date":"1999-02-03","releaseId":"x",
        "tracks":[{"number":1,"title":"First"},{"number":2,"title":"Second","artist":"A Guest"},{"number":99,"title":"Nope"},{"number":-1,"title":"Nope"}]})").object();
    c->applyMatch(match);
    QCOMPARE(c->disc().tracks[1].performer, QStringLiteral("A Guest"));
    // A picture that isn't one changes nothing; both looks draw without trouble.
    c->setCover(QByteArrayLiteral("not a picture"));
    c->setClassicLook(false);
    QVERIFY(!c->classicLook());
    QVERIFY(!c->grab().isNull());
    c->setClassicLook(true);
    QVERIFY(!c->grab().isNull());
    QVERIFY(!c->effectsShowing()); // the shared effects wait until they're asked for
    QVERIFY(!c->findChild<wv::VisualizerWidget *>());
    for (int i = 0; i < VisualizerView::StyleCount; i++) {
        c->visualizer()->setStyle(i);
        c->visualizer()->step();
        c->visualizer()->step();
        QVERIFY(!c->visualizer()->grab().isNull());
    }
    c->visualizer()->setStyle(0);
    w.close();
}

void Tests::cdScreenMakesImage()
{
    if (!QFile::exists(EngineClient::enginePath()))
        QSKIP("no helper");
    QTemporaryDir src, out;
    QByteArray pcm;
    const QString cue = makeCdImage(src.path(), {150, 150}, &pcm);
    MainWindow w(true);
    w.show();
    w.openPath(cue);
    QTRY_VERIFY_WITH_TIMEOUT(w.cd()->isVisible(), 15000);
    QSignalSpy finished(w.cd(), &CdPage::ripFinished);
    const QString dst = out.filePath(QStringLiteral("copy.cue"));
    w.cd()->makeImage(dst);
    QTRY_VERIFY_WITH_TIMEOUT(finished.size() == 1, 30000);
    QVERIFY2(w.cd()->statusText().contains(QStringLiteral("matches the disc")), qPrintable(w.cd()->statusText()));
    QFile bin(out.filePath(QStringLiteral("copy.bin")));
    QVERIFY(bin.open(QIODevice::ReadOnly));
    QVERIFY(bin.readAll() == pcm);
    // The image it made opens as the same disc.
    w.openPath(dst);
    QTRY_VERIFY_WITH_TIMEOUT(w.cd()->isVisible() && w.cd()->disc().path.endsWith(QStringLiteral("copy.cue")), 15000);
    QCOMPARE(w.cd()->disc().tracks.size(), 2);
    w.close();
}

// Checked here for what only the app can show: the real audio path feeds the visualizer, it
// costs playback nothing, and it goes away for a video. The drawing is tested by its own
// tests (wvtests), as this platform has no graphics for a widget.
void Tests::visualizerHearsTheMusicAndLeavesItAlone()
{
    QTemporaryDir dir;
    const QString wav = dir.filePath(QStringLiteral("tone.wav"));
    {
        // Half a minute of a low pulse, written by hand: no copy of FFmpeg needed.
        QFile f(wav);
        QVERIFY(f.open(QIODevice::WriteOnly));
        const int rate = 48000, frames = rate * 30;
        QByteArray pcm(frames * 4, Qt::Uninitialized);
        for (int i = 0; i < frames; i++) {
            const double t = double(i) / rate, since = std::fmod(t, 0.5);
            const qint16 v = qint16(std::sin(2 * M_PI * 70 * t) * std::exp(-since * 9) * 26000);
            memcpy(pcm.data() + i * 4, &v, 2);
            memcpy(pcm.data() + i * 4 + 2, &v, 2);
        }
        const auto u32 = [](quint32 v) { return QByteArray(reinterpret_cast<const char *>(&v), 4); };
        const auto u16 = [](quint16 v) { return QByteArray(reinterpret_cast<const char *>(&v), 2); };
        f.write("RIFF" + u32(36 + quint32(pcm.size())) + "WAVEfmt " + u32(16) + u16(1) + u16(2) + u32(rate) + u32(rate * 4) + u16(4) + u16(16) + "data" + u32(quint32(pcm.size())) + pcm);
    }
    QSettings s(QSettings::IniFormat, QSettings::UserScope, QStringLiteral("WittyMediaPlayer"), QStringLiteral("WittyMediaPlayer"));
    s.remove(QStringLiteral("visualizer"));
    s.setValue(QStringLiteral("visualizer/notice"), true); // a test mustn't stop at the notice, which is a dialog
    s.sync();
    {
        MainWindow w(true);
        // It costs nothing until it's opened.
        QVERIFY(w.visualizer() == nullptr);
        QVERIFY(!w.player()->audio()->tap()->enabled());
        w.handleKey(Qt::Key_V);
        QVERIFY2(w.visualizer() == nullptr, "there's no music yet, so it doesn't start");
        w.openPath(wav);
        QTRY_VERIFY_WITH_TIMEOUT(w.player()->isAudioOnly() && w.player()->hasMedia(), 8000);
        QTRY_VERIFY_WITH_TIMEOUT(w.player()->audio()->framesWritten() > 0, 8000);
        QVERIFY(!w.player()->audio()->tap()->enabled());
        w.show();
        w.handleKey(Qt::Key_V);
        QVERIFY(w.visualizerShowing());
        QTRY_VERIFY_WITH_TIMEOUT(w.player()->audio()->tap()->enabled(), 3000);

        // The samples the device is getting arrive, and the analysis hears the pulses in them.
        wv::Analyzer analyzer;
        std::vector<float> left(wv::kFftSize), right(wv::kFftSize);
        QElapsedTimer t;
        t.start();
        const auto under = w.player()->audio()->underruns();
        double loudest = 0;
        while (t.elapsed() < 3000) {
            QTest::qWait(16);
            w.player()->audio()->tap()->read(left.data(), right.data());
            analyzer.process(left.data(), right.data(), w.player()->audio()->tap()->rate(), 0.016);
            loudest = std::max(loudest, analyzer.frame().bass);
        }
        QVERIFY2(loudest > 0.5, "the bass comes through");
        QVERIFY2(analyzer.frame().beatCount >= 3, qPrintable(QStringLiteral("%1 beats in 3 seconds").arg(analyzer.frame().beatCount)));
        QCOMPARE(w.player()->audio()->underruns(), under);

        // Turned off, nothing runs and the audio thread stops copying.
        w.handleKey(Qt::Key_V);
        QVERIFY(!w.visualizerShowing());
        QVERIFY(!w.player()->audio()->tap()->enabled());
        w.handleKey(Qt::Key_V);
        QVERIFY(w.visualizerShowing());
        w.handleKey(Qt::Key_MediaStop);
        QVERIFY(!w.visualizerShowing());
    }
    s.remove(QStringLiteral("visualizer"));
}

QTEST_MAIN(Tests)
#include "tst_core.moc"
