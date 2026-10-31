// WittyMediaPlayer: a desktop media player for video, audio, DVDs and Blu-rays.
//
//   wittymediaplayer [file, disc image, disc folder or drive]
//
// --script runs a list of steps with no one at the keyboard, for automated checks
// (see testscript.cpp). It only reads from what it is told to play and writes only the
// files named on its command line.
#include "testscript.h"
#include "ui/mainwindow.h"

#include <QApplication>
#include <QCommandLineParser>
#include <QDir>
#include <QSettings>
#include <QTimer>

extern "C" {
#include <libavutil/log.h>
}

int main(int argc, char *argv[])
{
    QApplication::setApplicationName(QStringLiteral("WittyMediaPlayer"));
    QApplication::setOrganizationName(QStringLiteral("WittyMediaPlayer"));
    QApplication::setApplicationVersion(QStringLiteral(WMP_VERSION));
    QApplication app(argc, argv);

    QCommandLineParser args;
    args.setApplicationDescription(QStringLiteral("Plays video, audio, DVDs and Blu-rays."));
    args.addHelpOption();
    args.addVersionOption();
    args.addPositionalArgument(QStringLiteral("path"), QStringLiteral("A file, disc image, disc folder or drive to open."));
    QCommandLineOption scriptOpt(QStringLiteral("script"), QStringLiteral("Automated check: steps to run, separated by semicolons."), QStringLiteral("steps"));
    QCommandLineOption noAudioOpt(QStringLiteral("no-audio-device"), QStringLiteral("Don't use the sound card (for automated checks)."));
    QCommandLineOption hiddenOpt(QStringLiteral("off-screen-window"), QStringLiteral("Put the window where it can't be seen, and don't take the focus (for automated checks)."));
    args.addOption(scriptOpt);
    args.addOption(noAudioOpt);
    args.addOption(hiddenOpt);
    args.process(app);

    // An automated check leaves the viewer's own settings alone.
    if (args.isSet(scriptOpt))
        QSettings::setPath(QSettings::IniFormat, QSettings::UserScope, QDir::temp().filePath(QStringLiteral("wittymediaplayer-script-settings")));

    // FFmpeg's chatter stays out of the way unless asked for.
    av_log_set_level(qEnvironmentVariableIsSet("WMP_VERBOSE") ? AV_LOG_INFO : AV_LOG_QUIET);

    wmp::MainWindow window(args.isSet(noAudioOpt));
    if (args.isSet(hiddenOpt)) {
        window.setAttribute(Qt::WA_ShowWithoutActivating);
        window.setWindowFlag(Qt::Tool);
        window.setGeometry(-20000, -20000, 1280, 800);
    }
    window.show();

    const QStringList paths = args.positionalArguments();
    if (!paths.isEmpty())
        QTimer::singleShot(0, &window, [&window, p = paths.first()] { window.openPath(p); });
    if (args.isSet(scriptOpt))
        wmp::runScript(&window, args.value(scriptOpt));
    return app.exec();
}
