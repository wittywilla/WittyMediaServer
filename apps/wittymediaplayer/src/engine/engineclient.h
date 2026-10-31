// The window's end of the helper protocol: one line of JSON per message, with an
// optional binary blob after it, over the helper's stdin and stdout. Calls are
// answered in order. Sizes are checked before anything is read.
#pragma once

#include <QByteArray>
#include <QJsonObject>
#include <QJsonValue>
#include <QObject>
#include <QProcess>
#include <QString>
#include <deque>
#include <functional>

namespace wmp {

struct EngineReply {
    QJsonValue result;
    QByteArray blob;
    QString error;
    QString code;
    bool ok() const { return error.isEmpty(); }
};

class EngineClient : public QObject {
    Q_OBJECT
public:
    using Callback = std::function<void(const EngineReply &)>;

    explicit EngineClient(QObject *parent = nullptr);
    ~EngineClient() override;

    static constexpr qint64 kMaxLine = 1 << 20;
    static constexpr qint64 kMaxBlob = 128 << 20;

    // Starts the helper. Calling it again after a crash starts a new one.
    bool start();
    bool running() const;
    void stop();

    void call(const QString &method, const QJsonObject &params, Callback cb = {});

    // The engine program, next to this one.
    static QString enginePath();

signals:
    void helperLost();

private:
    void onReadable();
    void onFinished();
    bool parseOne();
    void failAll(const QString &message);

    QProcess *proc_ = nullptr;
    QByteArray buffer_;
    bool sawHello_ = false;
    bool stopping_ = false;
    quint64 next_ = 0;
    struct Pending {
        quint64 id;
        Callback cb;
    };
    std::deque<Pending> pending_;
    // A reply whose blob is still arriving.
    bool awaitingBlob_ = false;
    qint64 blobSize_ = 0;
    EngineReply partial_;
    quint64 partialId_ = 0;
};

} // namespace wmp
