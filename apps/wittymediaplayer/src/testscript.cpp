#include "testscript.h"

#include "disc/disccontroller.h"
#include "media/audiooutput.h"
#include "media/player.h"
#include "ui/mainwindow.h"
#include "video/videoview.h"

#include <QApplication>
#include <QFile>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QStringList>
#include <QTimer>
#include <memory>

namespace wmp {

namespace {

const QHash<QString, int> &keyNames()
{
    static const QHash<QString, int> names{
        {"up", Qt::Key_Up}, {"down", Qt::Key_Down}, {"left", Qt::Key_Left}, {"right", Qt::Key_Right}, {"enter", Qt::Key_Return},
        {"space", Qt::Key_Space}, {"m", Qt::Key_M}, {"t", Qt::Key_T}, {"a", Qt::Key_A}, {"s", Qt::Key_S}, {"n", Qt::Key_N}, {"p", Qt::Key_P},
        {"f", Qt::Key_F}, {"esc", Qt::Key_Escape}, {"backspace", Qt::Key_Backspace}, {"1", Qt::Key_1}, {"2", Qt::Key_2}, {"3", Qt::Key_3},
    };
    return names;
}

void writeReport(MainWindow *w, const QString &file)
{
    const PlayerStats st = w->player()->stats();
    DiscController *d = w->disc();
    QJsonObject o{
        {"position", w->player()->position()},
        {"duration", w->player()->duration()},
        {"paused", w->player()->paused()},
        {"framesDecoded", qint64(st.framesDecoded)},
        {"framesShown", qint64(st.framesShown)},
        {"framesDropped", qint64(st.framesDropped)},
        {"audioFrames", qint64(st.audioFrames)},
        {"underruns", qint64(st.underruns)},
        {"audioDevice", w->player()->audio()->hasDevice()},
        {"badBlocks", qint64(st.badBlocks)},
        {"width", st.width},
        {"height", st.height},
        {"hwDecode", st.hwDecode},
        {"deinterlacing", st.deinterlacing},
        {"videoCodec", st.videoCodec},
        {"audioCodec", st.audioCodec},
        {"status", w->statusText()},
        {"message", w->shownMessage()},
        {"discKind", d->active() ? d->presentationKind() : QString()},
        {"discTitle", d->ttn()},
        {"discButtons", w->menuButtonCount()},
        {"discSelected", d->selected()},
        {"cues", int(w->player()->activeCues().size())},
    };
    QJsonArray audio, subs;
    for (const TrackInfo &t : w->player()->audioTracks())
        audio.append(t.language);
    for (const TrackInfo &t : w->player()->subtitleTracks())
        subs.append(t.language.isEmpty() ? t.title : t.language);
    for (const DiscTrack &t : d->audioTracks())
        audio.append(t.language);
    for (const DiscTrack &t : d->subtitleTracks())
        subs.append(t.language);
    o.insert(QStringLiteral("audioTracks"), audio);
    o.insert(QStringLiteral("subtitleTracks"), subs);
    QFile f(file);
    if (f.open(QIODevice::WriteOnly))
        f.write(QJsonDocument(o).toJson());
}

struct Runner {
    MainWindow *window;
    QStringList steps;
    int at = 0;

    void next(std::shared_ptr<Runner> self)
    {
        if (at >= steps.size()) {
            qApp->quit();
            return;
        }
        const QString step = steps[at++].trimmed();
        const QString cmd = step.section(QLatin1Char(' '), 0, 0);
        const QString arg = step.section(QLatin1Char(' '), 1);
        int delay = 0;
        if (cmd == QLatin1String("wait")) {
            delay = int(arg.toDouble() * 1000);
        } else if (cmd == QLatin1String("open")) {
            window->openPath(arg);
        } else if (cmd == QLatin1String("key")) {
            window->handleKey(keyNames().value(arg, 0));
        } else if (cmd == QLatin1String("seek")) {
            if (window->disc()->active())
                window->disc()->seekTo(arg.toDouble());
            else
                window->player()->seek(arg.toDouble());
        } else if (cmd == QLatin1String("volume")) {
            window->player()->setVolume(arg.toDouble());
        } else if (cmd == QLatin1String("press")) {
            window->disc()->press(arg.toInt());
        } else if (cmd == QLatin1String("title")) {
            window->disc()->playTitle(arg.toInt());
        } else if (cmd == QLatin1String("mainfeature")) {
            window->disc()->playMain();
        } else if (cmd == QLatin1String("grab")) {
            window->view()->grabFramebuffer().save(arg);
        } else if (cmd == QLatin1String("windowgrab")) {
            window->grab().save(arg);
        } else if (cmd == QLatin1String("report")) {
            writeReport(window, arg);
        } else if (cmd == QLatin1String("quit")) {
            qApp->quit();
            return;
        }
        QTimer::singleShot(delay, window, [self] { self->next(self); });
    }
};

} // namespace

void runScript(MainWindow *window, const QString &steps)
{
    auto r = std::make_shared<Runner>();
    r->window = window;
    r->steps = steps.split(QLatin1Char(';'), Qt::SkipEmptyParts);
    QTimer::singleShot(0, window, [r] { r->next(r); });
}

} // namespace wmp
