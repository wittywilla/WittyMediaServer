#include "cdripper.h"

#include "engine/engineclient.h"

#include <QDateTime>
#include <QDir>
#include <QFile>
#include <QFileInfo>
#include <QJsonArray>
#include <QJsonDocument>
#include <QProcess>
#include <QRegularExpression>
#include <QThread>

namespace wmp {

namespace {

constexpr int kMaxPcm = 1 << 20;
constexpr int kMaxPart = 120; // characters in one folder or file name

QString safePart(QString s)
{
    static const QRegularExpression bad(QStringLiteral("[<>:\"/\\\\|?*\\x00-\\x1f]"));
    s.replace(bad, QStringLiteral("_"));
    s = s.simplified();
    while (s.endsWith(QLatin1Char('.')) || s.endsWith(QLatin1Char(' ')))
        s.chop(1);
    while (s.startsWith(QLatin1Char('.')))
        s.remove(0, 1);
    static const QRegularExpression reserved(QStringLiteral("^(con|prn|aux|nul|com[0-9]|lpt[0-9])(\\..*)?$"), QRegularExpression::CaseInsensitiveOption);
    if (reserved.match(s).hasMatch())
        s.prepend(QLatin1Char('_'));
    if (s.size() > kMaxPart)
        s = s.left(kMaxPart).trimmed();
    return s;
}

// A line of the helper's report, or false when it's gone.
bool readLine(QProcess &p, QByteArray *out, const std::atomic<bool> &stop, bool *closedInput)
{
    while (!p.canReadLine()) {
        if (stop.load() && !*closedInput) {
            p.closeWriteChannel(); // the helper stops when its input ends
            *closedInput = true;
        }
        if (!p.waitForReadyRead(200) && p.state() == QProcess::NotRunning && !p.canReadLine())
            return false;
    }
    *out = p.readLine(1 << 16);
    return true;
}

bool readBytes(QProcess &p, char *out, int n)
{
    int got = 0;
    while (got < n) {
        const qint64 r = p.read(out + got, n - got);
        if (r < 0)
            return false;
        got += int(r);
        if (got < n && !p.waitForReadyRead(2000) && p.state() == QProcess::NotRunning && p.bytesAvailable() == 0)
            return false;
    }
    return true;
}

QString writeReport(const RipJob &job, const QVector<RipResult> &results)
{
    QString folder;
    for (const RipResult &r : results)
        if (r.ok) {
            folder = QFileInfo(r.path).absolutePath();
            break;
        }
    if (folder.isEmpty())
        return {};
    const QString path = uniquePath(QDir(folder).filePath(QStringLiteral("%1 - copy report.txt").arg(safePart(job.album.isEmpty() ? QStringLiteral("CD") : job.album))));
    QFile f(path);
    if (!f.open(QIODevice::WriteOnly | QIODevice::NewOnly | QIODevice::Text))
        return {};
    QStringList l;
    l << QStringLiteral("WittyMediaPlayer %1, copied %2").arg(QStringLiteral(WMP_VERSION), QDateTime::currentDateTime().toString(Qt::ISODate));
    l << QStringLiteral("Format: %1").arg(ripFormat(job.format) ? ripFormat(job.format)->name : job.format);
    l << QStringLiteral("Reading: %1, drive offset %2 samples").arg(job.secure ? QStringLiteral("checked twice") : QStringLiteral("once")).arg(job.offset);
    l << QString();
    for (const RipResult &r : results) {
        if (r.ok)
            l << QStringLiteral("Track %1: %2. CRC %3, %4 retries, %5 unreadable sectors. %6").arg(r.number, 2, 10, QLatin1Char('0')).arg(r.quality, r.crc).arg(r.retries).arg(r.unreadable).arg(QFileInfo(r.path).fileName());
        else
            l << QStringLiteral("Track %1: not copied. %2").arg(r.number, 2, 10, QLatin1Char('0')).arg(r.error);
    }
    f.write(l.join(QLatin1Char('\n')).toUtf8());
    f.write("\n");
    return path;
}

} // namespace

CdInfo CdInfo::fromJson(const QJsonObject &o)
{
    CdInfo c;
    c.path = o.value(QStringLiteral("path")).toString();
    c.drive = o.value(QStringLiteral("drive")).toBool();
    c.title = o.value(QStringLiteral("title")).toString();
    c.performer = o.value(QStringLiteral("performer")).toString();
    c.date = o.value(QStringLiteral("date")).toString();
    c.discId = o.value(QStringLiteral("discId")).toString();
    c.toc = o.value(QStringLiteral("toc")).toString();
    for (const auto &v : o.value(QStringLiteral("warnings")).toArray())
        c.warnings << v.toString();
    for (const auto &v : o.value(QStringLiteral("tracks")).toArray()) {
        const QJsonObject t = v.toObject();
        if (!t.value(QStringLiteral("audio")).toBool())
            continue;
        CdTrack n;
        n.number = t.value(QStringLiteral("number")).toInt();
        n.start = t.value(QStringLiteral("start")).toInt();
        n.end = t.value(QStringLiteral("end")).toInt();
        n.title = t.value(QStringLiteral("title")).toString();
        n.performer = t.value(QStringLiteral("performer")).toString();
        n.isrc = t.value(QStringLiteral("isrc")).toString();
        n.preEmphasis = t.value(QStringLiteral("preEmphasis")).toBool();
        c.tracks.push_back(n);
    }
    return c;
}

QString defaultRipTemplate()
{
    return QStringLiteral("{albumartist}/{album}/{track} {title}");
}

QString ripRelativePath(const QString &nameTemplate, const RipNames &n, const QString &extension)
{
    const auto orElse = [](const QString &a, const QString &b) { return a.trimmed().isEmpty() ? b : a; };
    const QString trackNo = QStringLiteral("%1").arg(n.track, 2, 10, QLatin1Char('0'));
    const QString albumArtist = orElse(n.albumArtist, orElse(n.artist, QStringLiteral("Unknown artist")));
    const QList<QPair<QString, QString>> tokens = {
        {QStringLiteral("{albumartist}"), albumArtist},
        {QStringLiteral("{artist}"), orElse(n.artist, albumArtist)},
        {QStringLiteral("{album}"), orElse(n.album, QStringLiteral("Unknown album"))},
        {QStringLiteral("{title}"), orElse(n.title, QStringLiteral("Track %1").arg(trackNo))},
        {QStringLiteral("{track}"), trackNo},
        {QStringLiteral("{tracks}"), QString::number(n.tracks)},
        {QStringLiteral("{disc}"), n.disc > 0 ? QString::number(n.disc) : QString()},
        {QStringLiteral("{year}"), n.year.left(4)},
    };
    // The template's own slashes make folders. A slash in a song's name doesn't.
    QStringList parts;
    const QString shape = QString(nameTemplate).replace(QLatin1Char('\\'), QLatin1Char('/'));
    for (QString part : shape.split(QLatin1Char('/'), Qt::SkipEmptyParts)) {
        for (const auto &t : tokens)
            part.replace(t.first, safePart(t.second));
        part = safePart(part);
        if (!part.isEmpty())
            parts << part;
    }
    if (parts.isEmpty())
        parts << QStringLiteral("Track %1").arg(trackNo);
    return parts.join(QLatin1Char('/')) + QLatin1Char('.') + extension;
}

QString uniquePath(const QString &path)
{
    if (!QFileInfo::exists(path) && !QFileInfo::exists(path + QStringLiteral(".part")))
        return path;
    const QFileInfo fi(path);
    for (int i = 2; i < 1000; i++) {
        const QString next = fi.dir().filePath(QStringLiteral("%1 (%2).%3").arg(fi.completeBaseName()).arg(i).arg(fi.suffix()));
        if (!QFileInfo::exists(next) && !QFileInfo::exists(next + QStringLiteral(".part")))
            return next;
    }
    return {};
}

CdRipper::CdRipper(QObject *parent) : QObject(parent)
{
    qRegisterMetaType<wmp::RipResult>();
}

CdRipper::~CdRipper()
{
    stop();
    if (thread_) {
        thread_->wait();
        delete thread_;
    }
}

void CdRipper::start(const RipJob &job)
{
    if (thread_)
        return;
    stop_.store(false);
    thread_ = QThread::create([this, job] { run(job); });
    connect(thread_, &QThread::finished, this, [this] {
        thread_->deleteLater();
        thread_ = nullptr;
    });
    thread_->start();
}

void CdRipper::stop()
{
    stop_.store(true);
}

void CdRipper::run(const RipJob &job)
{
    QVector<RipResult> results;
    const RipFormat *format = ripFormat(job.format);
    if (!format) {
        emit finished(false, QStringLiteral("This player doesn't write that format."), {});
        return;
    }
    QStringList numbers;
    for (const RipTrackJob &t : job.tracks)
        numbers << QString::number(t.number);
    QProcess p;
    p.setStandardErrorFile(QProcess::nullDevice());
    p.start(EngineClient::enginePath(),
            {QStringLiteral("--watch-stdin"), QStringLiteral("--cd-rip"), job.source, numbers.join(QLatin1Char(',')),
             job.secure ? QStringLiteral("secure") : QStringLiteral("burst"), QString::number(job.offset)},
            QIODevice::ReadWrite);
    if (!p.waitForStarted(10000)) {
        emit finished(false, QStringLiteral("WittyMediaPlayer's helper program didn't start."), {});
        return;
    }

    AudioEncoder encoder;
    QString finalPath, partPath;
    RipResult current;
    bool open = false;
    QString trouble;
    bool closedInput = false;
    QByteArray pcm(kMaxPcm, Qt::Uninitialized);

    const auto dropPartial = [&] {
        encoder.close();
        if (!partPath.isEmpty())
            QFile::remove(partPath);
        partPath.clear();
        open = false;
    };

    QByteArray line;
    while (readLine(p, &line, stop_, &closedInput)) {
        const QJsonObject ev = QJsonDocument::fromJson(line).object();
        const QString event = ev.value(QStringLiteral("event")).toString();
        const int number = ev.value(QStringLiteral("track")).toInt();
        if (event == QLatin1String("track")) {
            const RipTrackJob *t = nullptr;
            for (const RipTrackJob &j : job.tracks)
                if (j.number == number)
                    t = &j;
            current = RipResult{};
            current.number = number;
            emit trackStarted(number);
            if (!t) {
                trouble = QStringLiteral("The helper program sent a track that wasn't asked for.");
                break;
            }
            finalPath = uniquePath(QDir(job.folder).filePath(ripRelativePath(job.nameTemplate, t->names, format->extension)));
            if (finalPath.isEmpty() || !QDir().mkpath(QFileInfo(finalPath).absolutePath())) {
                trouble = QStringLiteral("The folder for track %1 couldn't be made.").arg(number);
                break;
            }
            partPath = finalPath + QStringLiteral(".part");
            QString why;
            if (!encoder.open(partPath, job.format, job.quality, t->tags, &why)) {
                trouble = QStringLiteral("Track %1: %2").arg(number).arg(why);
                QFile::remove(partPath);
                partPath.clear();
                break;
            }
            open = true;
        } else if (event == QLatin1String("progress")) {
            emit trackProgress(number, ev.value(QStringLiteral("percent")).toInt());
        } else if (event == QLatin1String("pcm")) {
            const int bytes = ev.value(QStringLiteral("bytes")).toInt();
            if (bytes <= 0 || bytes > kMaxPcm || !open || !readBytes(p, pcm.data(), bytes)) {
                trouble = QStringLiteral("The helper program stopped sending this track.");
                break;
            }
            if (stop_.load())
                continue;
            if (!encoder.write(pcm.constData(), bytes)) {
                trouble = QStringLiteral("Track %1: %2").arg(number).arg(encoder.error());
                break;
            }
        } else if (event == QLatin1String("done")) {
            const QJsonObject rep = ev.value(QStringLiteral("report")).toObject();
            current.crc = rep.value(QStringLiteral("crc")).toString();
            current.verified = rep.value(QStringLiteral("verified")).toBool();
            current.retries = rep.value(QStringLiteral("retries")).toInt();
            current.unreadable = rep.value(QStringLiteral("unreadable")).toInt();
            current.quality = ev.value(QStringLiteral("quality")).toString();
            if (!open || !encoder.finish()) {
                trouble = QStringLiteral("Track %1: %2").arg(number).arg(encoder.error());
                break;
            }
            open = false;
            if (!QFile::rename(partPath, finalPath)) {
                trouble = QStringLiteral("Track %1 couldn't be given its name.").arg(number);
                break;
            }
            partPath.clear();
            current.ok = true;
            current.path = finalPath;
            results.push_back(current);
            emit trackFinished(current);
        } else if (event == QLatin1String("error")) {
            const QString code = ev.value(QStringLiteral("code")).toString();
            const QString message = ev.value(QStringLiteral("message")).toString();
            dropPartial();
            if (code == QLatin1String("cancelled"))
                break;
            if (number > 0) {
                current.number = number;
                current.ok = false;
                current.error = message;
                results.push_back(current);
                emit trackFinished(current);
            }
            if (code != QLatin1String("unreadable"))
                trouble = message;
            else if (trouble.isEmpty())
                trouble = QStringLiteral("Some tracks couldn't be read. The rest were copied.");
        } else if (event == QLatin1String("finished")) {
            break;
        }
    }
    dropPartial();
    p.closeWriteChannel();
    if (!p.waitForFinished(5000)) {
        p.kill();
        p.waitForFinished(2000);
    }
    const QString report = writeReport(job, results);
    emit finished(stop_.load(), stop_.load() ? QString() : trouble, report);
}

} // namespace wmp
