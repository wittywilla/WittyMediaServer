// The player's settings: which libraries play things.
#pragma once

#include <QDialog>
#include <QHash>
#include <QJsonObject>
#include <functional>

class QCheckBox;
class QLabel;
class QLineEdit;
class QPushButton;
class QRadioButton;
class QVBoxLayout;

namespace wmp {

class EngineClient;
class Player;

class SettingsDialog : public QDialog {
    Q_OBJECT
public:
    SettingsDialog(Player *player, EngineClient *engine, bool useVlc, const QString &vlcFolder, bool checkModules, QWidget *parent = nullptr);
    bool checkModulesAtStart() const;

    bool useVlc() const;
    QString vlcFolder() const;

    // What to say about the VLC libraries, wherever they're credited.
    static QString vlcCredit();

private:
    void refresh();
    void loadModules();
    void checkForUpdates();
    void runUpdater(const QStringList &args, const std::function<void(const QJsonObject &)> &done);

    Player *player_;
    EngineClient *engine_;
    QVBoxLayout *moduleRows_;
    QLabel *moduleStatus_;
    QPushButton *checkBtn_;
    QCheckBox *checkAtStart_;
    QHash<QString, QString> offers_;
    QRadioButton *builtin_, *vlc_;
    QLineEdit *folder_;
    QLabel *status_;
};

} // namespace wmp
