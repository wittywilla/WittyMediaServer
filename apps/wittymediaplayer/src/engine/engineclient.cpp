#include "engineclient.h"

#include <QCoreApplication>
#include <QDir>
#include <QJsonDocument>

namespace wmp {

EngineClient::EngineClient(QObject *parent) : QObject(parent) {}

EngineClient::~EngineClient()
{
    stop();
}

QString EngineClient::enginePath()
{
    QString name = QStringLiteral("wittymediaplayer-engine");
#ifdef Q_OS_WIN
    name += QStringLiteral(".exe");
#endif
    return QDir(QCoreApplication::applicationDirPath()).filePath(name);
}

bool EngineClient::start()
{
    if (running())
        return true;
    stopping_ = false;
    buffer_.clear();
    sawHello_ = false;
    awaitingBlob_ = false;
    delete proc_;
    proc_ = new QProcess(this);
    proc_->setProcessChannelMode(QProcess::SeparateChannels);
    proc_->setStandardErrorFile(QProcess::nullDevice());
    connect(proc_, &QProcess::readyReadStandardOutput, this, &EngineClient::onReadable);
    connect(proc_, &QProcess::finished, this, &EngineClient::onFinished);
    connect(proc_, &QProcess::errorOccurred, this, [this](QProcess::ProcessError e) {
        if (e == QProcess::FailedToStart)
            onFinished();
    });
    proc_->start(enginePath(), {QStringLiteral("--helper")}, QIODevice::ReadWrite);
    return proc_->waitForStarted(5000);
}

bool EngineClient::running() const
{
    return proc_ && proc_->state() != QProcess::NotRunning;
}

void EngineClient::stop()
{
    stopping_ = true;
    if (proc_ && proc_->state() != QProcess::NotRunning) {
        // Closing its stdin is how the helper knows to leave.
        proc_->closeWriteChannel();
        if (!proc_->waitForFinished(1500)) {
            proc_->kill();
            proc_->waitForFinished(500);
        }
    }
}

void EngineClient::call(const QString &method, const QJsonObject &params, Callback cb)
{
    if (!running() && !start()) {
        EngineReply r;
        r.error = QStringLiteral("WittyMediaPlayer's helper program couldn't start. Try reinstalling it.");
        if (cb)
            cb(r);
        return;
    }
    QJsonObject req{{QStringLiteral("id"), qint64(++next_)}, {QStringLiteral("method"), method}};
    if (!params.isEmpty())
        req.insert(QStringLiteral("params"), params);
    pending_.push_back({next_, std::move(cb)});
    proc_->write(QJsonDocument(req).toJson(QJsonDocument::Compact) + '\n');
}

void EngineClient::onReadable()
{
    buffer_.append(proc_->readAllStandardOutput());
    while (parseOne()) {
    }
}

bool EngineClient::parseOne()
{
    if (awaitingBlob_) {
        if (buffer_.size() < blobSize_)
            return false;
        partial_.blob = buffer_.left(blobSize_);
        buffer_.remove(0, blobSize_);
        awaitingBlob_ = false;
        for (auto it = pending_.begin(); it != pending_.end(); ++it) {
            if (it->id == partialId_) {
                Callback cb = std::move(it->cb);
                pending_.erase(it);
                if (cb)
                    cb(partial_);
                break;
            }
        }
        partial_ = {};
        return true;
    }
    const int nl = buffer_.indexOf('\n');
    if (nl < 0) {
        if (buffer_.size() > kMaxLine) {
            // Not a protocol this side speaks: end it rather than keep buffering.
            proc_->kill();
            buffer_.clear();
        }
        return false;
    }
    const QByteArray line = buffer_.left(nl);
    buffer_.remove(0, nl + 1);
    const QJsonObject o = QJsonDocument::fromJson(line).object();
    if (!sawHello_) {
        sawHello_ = true;
        return true;
    }
    EngineReply r;
    r.result = o.value(QStringLiteral("result"));
    r.error = o.value(QStringLiteral("error")).toString();
    r.code = o.value(QStringLiteral("code")).toString();
    const qint64 blob = qint64(o.value(QStringLiteral("blob")).toDouble());
    const quint64 id = quint64(o.value(QStringLiteral("id")).toDouble());
    if (blob < 0 || blob > kMaxBlob) {
        proc_->kill();
        return false;
    }
    if (blob > 0) {
        partial_ = r;
        partialId_ = id;
        blobSize_ = blob;
        awaitingBlob_ = true;
        return true;
    }
    for (auto it = pending_.begin(); it != pending_.end(); ++it) {
        if (it->id == id) {
            Callback cb = std::move(it->cb);
            pending_.erase(it);
            if (cb)
                cb(r);
            break;
        }
    }
    return true;
}

void EngineClient::failAll(const QString &message)
{
    auto list = std::move(pending_);
    pending_.clear();
    for (auto &p : list) {
        if (p.cb) {
            EngineReply r;
            r.error = message;
            r.code = QStringLiteral("helper-lost");
            p.cb(r);
        }
    }
}

void EngineClient::onFinished()
{
    failAll(QStringLiteral("WittyMediaPlayer's disc helper stopped unexpectedly. This disc may be damaged."));
    if (!stopping_)
        emit helperLost();
}

} // namespace wmp
