#include "settingsdialog.h"

#include "../engine/engineclient.h"
#include "../media/player.h"
#include "../media/vlcplayback.h"

#include <QCheckBox>
#include <QCoreApplication>
#include <QDialogButtonBox>
#include <QDir>
#include <QFileDialog>
#include <QFrame>
#include <QHBoxLayout>
#include <QJsonArray>
#include <QJsonDocument>
#include <QLabel>
#include <QLineEdit>
#include <QProcess>
#include <QPushButton>
#include <QRadioButton>
#include <QVBoxLayout>

namespace wmp {

QString SettingsDialog::vlcCredit()
{
    return QStringLiteral("VLC and libVLC are made by VideoLAN and the VLC contributors (videolan.org), and are free software "
                          "under the LGPL 2.1 or later. Thank you to them. WittyMediaPlayer isn't part of VLC or made by VideoLAN.");
}

SettingsDialog::SettingsDialog(Player *player, EngineClient *engine, bool useVlc, const QString &vlcFolder, bool checkModules, QWidget *parent)
    : QDialog(parent), player_(player), engine_(engine)
{
    setWindowTitle(QStringLiteral("Settings"));
    setMinimumWidth(520);
    auto *col = new QVBoxLayout(this);
    col->setContentsMargins(20, 18, 20, 18);
    col->setSpacing(10);

    auto *heading = new QLabel(QStringLiteral("What plays your media"), this);
    heading->setProperty("role", QStringLiteral("heading"));
    col->addWidget(heading);

    builtin_ = new QRadioButton(QStringLiteral("Built in (FFmpeg, built for this player)"), this);
    vlc_ = new QRadioButton(QStringLiteral("VLC libraries, from a VLC on this computer"), this);
    builtin_->setChecked(!useVlc);
    vlc_->setChecked(useVlc);
    col->addWidget(builtin_);
    auto *builtinNote = new QLabel(QStringLiteral("The lightest choice, and the one tested most. Subtitles and menus are at their best here."), this);
    builtinNote->setProperty("role", QStringLiteral("hint"));
    builtinNote->setWordWrap(true);
    col->addWidget(builtinNote);
    col->addWidget(vlc_);
    auto *vlcNote = new QLabel(QStringLiteral("Useful for a format the built-in choice can't play, and it plays surround sound. Disc menus still come from "
                                              "WittyDVD and WittyBluray, and copy-protected discs are still refused: VLC's own disc modules aren't used."), this);
    vlcNote->setProperty("role", QStringLiteral("hint"));
    vlcNote->setWordWrap(true);
    col->addWidget(vlcNote);

    auto *row = new QHBoxLayout;
    folder_ = new QLineEdit(vlcFolder, this);
    folder_->setAccessibleName(QStringLiteral("VLC's folder"));
    folder_->setPlaceholderText(QStringLiteral("VLC's folder"));
    auto *browse = new QPushButton(QStringLiteral("Choose…"), this);
    browse->setAccessibleName(QStringLiteral("Choose VLC's folder"));
    row->addWidget(folder_, 1);
    row->addWidget(browse);
    col->addLayout(row);
    status_ = new QLabel(this);
    status_->setProperty("role", QStringLiteral("muted"));
    status_->setWordWrap(true);
    status_->setAccessibleName(QStringLiteral("VLC status"));
    col->addWidget(status_);
    auto *credit = new QLabel(vlcCredit(), this);
    credit->setProperty("role", QStringLiteral("hint"));
    credit->setWordWrap(true);
    col->addWidget(credit);

    auto *line = new QFrame(this);
    line->setFrameShape(QFrame::HLine);
    col->addWidget(line);
    auto *modulesHeading = new QLabel(QStringLiteral("Disc modules"), this);
    modulesHeading->setProperty("role", QStringLiteral("heading"));
    col->addWidget(modulesHeading);
    auto *modulesNote = new QLabel(QStringLiteral("WittyDVD and WittyBluray can each be updated without a new player. Only versions signed by this project run, and "
                                                  "the built-in copy is always there if one doesn't work. Looking for updates is the only time this player uses the internet, and it "
                                                  "talks only to GitHub."), this);
    modulesNote->setProperty("role", QStringLiteral("hint"));
    modulesNote->setWordWrap(true);
    col->addWidget(modulesNote);
    moduleRows_ = new QVBoxLayout;
    moduleRows_->setSpacing(6);
    col->addLayout(moduleRows_);
    moduleStatus_ = new QLabel(this);
    moduleStatus_->setProperty("role", QStringLiteral("muted"));
    moduleStatus_->setWordWrap(true);
    moduleStatus_->setAccessibleName(QStringLiteral("Module updates status"));
    col->addWidget(moduleStatus_);
    auto *moduleButtons = new QHBoxLayout;
    checkBtn_ = new QPushButton(QStringLiteral("Look for updates now"), this);
    checkAtStart_ = new QCheckBox(QStringLiteral("Look when the player starts"), this);
    checkAtStart_->setChecked(checkModules);
    moduleButtons->addWidget(checkBtn_);
    moduleButtons->addWidget(checkAtStart_);
    moduleButtons->addStretch(1);
    col->addLayout(moduleButtons);
    connect(checkBtn_, &QPushButton::clicked, this, &SettingsDialog::checkForUpdates);
    loadModules();

    auto *buttons = new QDialogButtonBox(QDialogButtonBox::Ok | QDialogButtonBox::Cancel, this);
    col->addWidget(buttons);
    connect(buttons, &QDialogButtonBox::accepted, this, &QDialog::accept);
    connect(buttons, &QDialogButtonBox::rejected, this, &QDialog::reject);
    connect(browse, &QPushButton::clicked, this, [this] {
        const QString d = QFileDialog::getExistingDirectory(this, QStringLiteral("Choose VLC's folder"), folder_->text());
        if (!d.isEmpty()) {
            folder_->setText(QDir::toNativeSeparators(d));
            refresh();
        }
    });
    connect(vlc_, &QRadioButton::toggled, this, &SettingsDialog::refresh);
    connect(folder_, &QLineEdit::editingFinished, this, &SettingsDialog::refresh);
    refresh();
}

bool SettingsDialog::checkModulesAtStart() const
{
    return checkAtStart_->isChecked();
}

void SettingsDialog::loadModules()
{
    if (!engine_)
        return;
    engine_->call(QStringLiteral("modules"), {}, [this](const EngineReply &r) {
        if (!r.ok())
            return;
        while (QLayoutItem *it = moduleRows_->takeAt(0)) {
            delete it->widget();
            delete it;
        }
        for (const auto &v : r.result.toArray()) {
            const QJsonObject o = v.toObject();
            const QString id = o.value("id").toString(), name = o.value("name").toString();
            auto *row = new QWidget(this);
            row->setObjectName(QStringLiteral("driveRow"));
            row->setAttribute(Qt::WA_StyledBackground);
            auto *rowL = new QHBoxLayout(row);
            rowL->setContentsMargins(12, 8, 8, 8);
            QString text = QStringLiteral("%1: %2").arg(name, o.value("inUse").toString());
            text += o.value("isBuiltin").toBool() ? QStringLiteral(" (built in)") : QStringLiteral(" (updated)");
            const QString note = o.value("note").toString();
            if (!note.isEmpty())
                text += QLatin1Char('\n') + note;
            auto *label = new QLabel(text, row);
            label->setWordWrap(true);
            rowL->addWidget(label, 1);
            const QString offered = offers_.value(id);
            if (!offered.isEmpty()) {
                auto *install = new QPushButton(QStringLiteral("Install %1").arg(offered), row);
                install->setAccessibleName(QStringLiteral("Install %1 version %2").arg(name, offered));
                connect(install, &QPushButton::clicked, this, [this, id, offered, name] {
                    moduleStatus_->setText(QStringLiteral("Installing %1 %2…").arg(name, offered));
                    runUpdater({QStringLiteral("--install"), id, offered}, [this, id, name, offered](const QJsonObject &res) {
                        const QString err = res.value("error").toString();
                        moduleStatus_->setText(err.isEmpty() ? QStringLiteral("%1 %2 is installed, and takes over the next time you open a disc.").arg(name, offered)
                                                             : QStringLiteral("%1 wasn't updated: %2").arg(name, err));
                        if (err.isEmpty())
                            offers_.remove(id);
                        loadModules();
                    });
                });
                rowL->addWidget(install);
            }
            if (o.value("canGoBack").toBool()) {
                auto *back = new QPushButton(QStringLiteral("Go back"), row);
                back->setAccessibleName(QStringLiteral("Go back to the earlier version of %1").arg(name));
                connect(back, &QPushButton::clicked, this, [this, id, name] {
                    engine_->call(QStringLiteral("module-rollback"), {{"id", id}}, [this, name](const EngineReply &rb) {
                        moduleStatus_->setText(rb.ok() ? QStringLiteral("%1 is back to %2, from the next disc you open.").arg(name, rb.result.toObject().value("inUse").toString()) : rb.error);
                        loadModules();
                    });
                });
                rowL->addWidget(back);
            }
            moduleRows_->addWidget(row);
        }
    });
}

void SettingsDialog::runUpdater(const QStringList &args, const std::function<void(const QJsonObject &)> &done)
{
    checkBtn_->setEnabled(false);
    auto *proc = new QProcess(this);
    proc->setStandardErrorFile(QProcess::nullDevice());
    connect(proc, &QProcess::finished, this, [this, proc, done] {
        checkBtn_->setEnabled(true);
        const QJsonObject o = QJsonDocument::fromJson(proc->readAllStandardOutput()).object();
        proc->deleteLater();
        done(o.isEmpty() ? QJsonObject{{"error", QStringLiteral("the updater stopped unexpectedly")}} : o);
    });
    QString updater = QStringLiteral("wittymediaplayer-updater");
#ifdef Q_OS_WIN
    updater += QStringLiteral(".exe");
#endif
    proc->start(QDir(QCoreApplication::applicationDirPath()).filePath(updater), args, QIODevice::ReadOnly);
}

void SettingsDialog::checkForUpdates()
{
    moduleStatus_->setText(QStringLiteral("Looking on GitHub…"));
    runUpdater({QStringLiteral("--check")}, [this](const QJsonObject &res) {
        offers_.clear();
        QStringList notes;
        const QString err = res.value("error").toString();
        if (!err.isEmpty())
            notes << err;
        for (const auto &v : res.value("offers").toArray()) {
            const QJsonObject o = v.toObject();
            if (!o.value("version").toString().isEmpty())
                offers_.insert(o.value("id").toString(), o.value("version").toString());
            if (!o.value("note").toString().isEmpty())
                notes << o.value("note").toString();
        }
        if (notes.isEmpty())
            notes << (offers_.isEmpty() ? QStringLiteral("Everything is up to date.") : QStringLiteral("There are updates to install."));
        moduleStatus_->setText(notes.join(QLatin1Char(' ')));
        loadModules();
    });
}

bool SettingsDialog::useVlc() const
{
    return vlc_->isChecked();
}

QString SettingsDialog::vlcFolder() const
{
    return folder_->text().trimmed();
}

void SettingsDialog::refresh()
{
    folder_->setEnabled(vlc_->isChecked());
    if (!vlc_->isChecked()) {
        status_->clear();
        return;
    }
    QString why;
    if (player_->enableVlc(vlcFolder(), &why))
        status_->setText(QStringLiteral("Found VLC %1. It takes effect the next time you open something.").arg(player_->vlcVersion()));
    else if (vlcFolder().isEmpty())
        status_->setText(QStringLiteral("VLC wasn't found on this computer. Install it from videolan.org, or choose its folder."));
    else
        status_->setText(why);
}

} // namespace wmp
