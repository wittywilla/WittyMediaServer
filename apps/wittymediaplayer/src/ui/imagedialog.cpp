#include "imagedialog.h"

#include "../engine/engineclient.h"

#include <QComboBox>
#include <QDir>
#include <QFileDialog>
#include <QFileInfo>
#include <QHBoxLayout>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QLabel>
#include <QProgressBar>
#include <QPushButton>
#include <QRegularExpression>
#include <QVBoxLayout>

namespace wmp {

namespace {
QString sizeText(double bytes)
{
    return bytes >= 1024.0 * 1024 * 1024 ? QStringLiteral("%1 GB").arg(bytes / (1024.0 * 1024 * 1024), 0, 'f', 1)
                                         : QStringLiteral("%1 MB").arg(bytes / (1024.0 * 1024), 0, 'f', 0);
}
} // namespace

QString ImageDialog::suggestedName(const QString &label, const QString &extension)
{
    QString name = label.trimmed();
    static const QRegularExpression bad(QStringLiteral("[<>:\"/\\\\|?*\\x00-\\x1f]"));
    name.replace(bad, QStringLiteral("_"));
    while (name.endsWith(QLatin1Char('.')) || name.endsWith(QLatin1Char(' ')))
        name.chop(1);
    if (name.isEmpty())
        name = QStringLiteral("Disc");
    return name + QLatin1Char('.') + extension;
}

ImageDialog::ImageDialog(EngineClient *engine, const QString &source, const QString &lastDir, QWidget *parent)
    : QDialog(parent), engine_(engine), source_(source), lastDir_(lastDir)
{
    setWindowTitle(QStringLiteral("Make a disc image"));
    setMinimumWidth(480);
    auto *col = new QVBoxLayout(this);
    col->setContentsMargins(20, 18, 20, 18);
    col->setSpacing(12);
    auto *intro = new QLabel(QStringLiteral("Copies a disc to one file you can keep, play later, or add to a WittyMediaServer library. Copy-protected discs can't be copied."), this);
    intro->setWordWrap(true);
    col->addWidget(intro);
    drives_ = new QComboBox(this);
    drives_->setAccessibleName(QStringLiteral("Disc to copy"));
    col->addWidget(drives_);
    status_ = new QLabel(this);
    status_->setProperty("role", QStringLiteral("muted"));
    status_->setWordWrap(true);
    status_->setAccessibleName(QStringLiteral("Progress"));
    col->addWidget(status_);
    progress_ = new QProgressBar(this);
    progress_->setRange(0, 1000);
    progress_->setTextVisible(false);
    progress_->hide();
    col->addWidget(progress_);
    auto *row = new QHBoxLayout;
    row->addStretch(1);
    cancelBtn_ = new QPushButton(QStringLiteral("Close"), this);
    startBtn_ = new QPushButton(QStringLiteral("Choose where to save…"), this);
    startBtn_->setProperty("role", QStringLiteral("primary"));
    row->addWidget(cancelBtn_);
    row->addWidget(startBtn_);
    col->addLayout(row);
    connect(startBtn_, &QPushButton::clicked, this, &ImageDialog::chooseAndStart);
    connect(cancelBtn_, &QPushButton::clicked, this, &ImageDialog::reject);
    loadDrives();
}

ImageDialog::~ImageDialog()
{
    if (proc_ && proc_->state() != QProcess::NotRunning) {
        proc_->closeWriteChannel();
        if (!proc_->waitForFinished(5000))
            proc_->kill();
    }
}

void ImageDialog::loadDrives()
{
    drives_->clear();
    const QString suffix = QFileInfo(source_).suffix().toLower();
    if (suffix == QLatin1String("iso") || suffix == QLatin1String("img"))
        source_.clear(); // already an image
    engine_->call(QStringLiteral("drives"), {}, [this](const EngineReply &r) {
        if (!r.ok())
            return;
        int pick = -1;
        const QJsonArray list = r.result.toArray();
        for (const auto &v : list) {
            const QJsonObject d = v.toObject();
            if (!d.value("media").toBool() || d.value("audio").toBool())
                continue;
            const QString path = d.value("path").toString();
            drives_->addItem(QStringLiteral("%1 (%2)").arg(d.value("name").toString(), d.value("label").toString()), QJsonObject{{"path", path}, {"label", d.value("label")}});
            if (!source_.isEmpty() && QDir::toNativeSeparators(source_).startsWith(path, Qt::CaseInsensitive))
                pick = drives_->count() - 1;
        }
        if (pick >= 0)
            drives_->setCurrentIndex(pick);
        startBtn_->setEnabled(drives_->count() > 0);
        status_->setText(drives_->count() > 0 ? QString() : QStringLiteral("There's no disc to copy. Put one in a drive and open this again."));
    });
}

void ImageDialog::chooseAndStart()
{
    if (drives_->count() == 0)
        return;
    const QJsonObject d = drives_->currentData().toJsonObject();
    const QString name = suggestedName(d.value("label").toString(), QStringLiteral("iso"));
    const QString dir = lastDir_.isEmpty() ? QDir::homePath() : lastDir_;
    const QString out = QFileDialog::getSaveFileName(this, QStringLiteral("Save the disc image"), QDir(dir).filePath(name),
        QStringLiteral("ISO image (*.iso);;IMG image (*.img)"), nullptr, QFileDialog::DontConfirmOverwrite);
    if (!out.isEmpty())
        start(out);
}

void ImageDialog::start(const QString &out)
{
    if (QFileInfo::exists(out)) {
        status_->setText(QStringLiteral("A file with that name is already there, and copying a disc never overwrites anything. Pick another name."));
        return;
    }
    out_ = out;
    lastDir_ = QFileInfo(out).absolutePath();
    emit folderChosen(lastDir_);
    const QJsonObject d = drives_->currentData().toJsonObject();
    startBtn_->setEnabled(false);
    drives_->setEnabled(false);
    cancelBtn_->setText(QStringLiteral("Stop"));
    progress_->show();
    progress_->setValue(0);
    status_->setText(QStringLiteral("Checking the disc…"));
    proc_ = new QProcess(this);
    proc_->setStandardErrorFile(QProcess::nullDevice());
    connect(proc_, &QProcess::readyReadStandardOutput, this, &ImageDialog::onOutput);
    connect(proc_, &QProcess::finished, this, &ImageDialog::onFinished);
    proc_->start(EngineClient::enginePath(), {QStringLiteral("--watch-stdin"), QStringLiteral("--image"), d.value("path").toString(), QDir::toNativeSeparators(out)}, QIODevice::ReadWrite);
}

void ImageDialog::onOutput()
{
    buffer_.append(proc_->readAllStandardOutput());
    for (int nl = buffer_.indexOf('\n'); nl >= 0; nl = buffer_.indexOf('\n')) {
        const QJsonObject o = QJsonDocument::fromJson(buffer_.left(nl)).object();
        buffer_.remove(0, nl + 1);
        const QString ev = o.value("event").toString();
        const double done = o.value("done").toDouble(), total = o.value("total").toDouble();
        if (ev == QLatin1String("progress")) {
            progress_->setValue(total > 0 ? int(1000 * done / total) : 0);
            QString text = QStringLiteral("%1 of %2").arg(sizeText(done), sizeText(total));
            const double speed = o.value("bytesPerSec").toDouble();
            if (speed > 0)
                text += QStringLiteral(", about %1 minutes left").arg(qMax(1, int((total - done) / speed / 60 + 0.5)));
            const int bad = o.value("badSectors").toInt();
            if (bad > 0)
                text += QStringLiteral(". %1 unreadable so far").arg(bad);
            status_->setText(text);
        } else if (ev == QLatin1String("done")) {
            finished_ = true;
            progress_->setValue(1000);
            QString text = QStringLiteral("Saved %1.").arg(QDir::toNativeSeparators(out_));
            const int bad = o.value("badSectors").toInt();
            if (bad > 0)
                text += QStringLiteral(" %1 sectors couldn't be read and are blank, so parts of it may not play.").arg(bad);
            if (o.value("verified").toBool())
                text += QStringLiteral(" WittyMediaServer's disc engines read it fine: put it in a folder your disc library watches and scan.");
            else
                text += QStringLiteral(" The disc engines couldn't read it back, so a library may not be able to use it.");
            status_->setText(text);
        } else if (ev == QLatin1String("error")) {
            finished_ = true;
            progress_->hide();
            status_->setText(o.value("message").toString());
        }
    }
}

void ImageDialog::onFinished()
{
    if (!finished_ && proc_->exitStatus() != QProcess::NormalExit)
        status_->setText(QStringLiteral("The copy stopped unexpectedly."));
    cancelBtn_->setText(QStringLiteral("Close"));
    startBtn_->setEnabled(drives_->count() > 0);
    drives_->setEnabled(true);
    finished_ = false;
}

void ImageDialog::reject()
{
    if (proc_ && proc_->state() != QProcess::NotRunning) {
        // Closing its stdin stops the copy and removes the partial file.
        status_->setText(QStringLiteral("Stopping…"));
        proc_->closeWriteChannel();
        return;
    }
    QDialog::reject();
}

} // namespace wmp
