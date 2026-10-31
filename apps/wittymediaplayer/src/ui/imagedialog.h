// Makes an .iso or .img file from a disc, for a WittyMediaServer library or for
// playing later. The copy runs in the helper program, which refuses protected discs.
#pragma once

#include <QDialog>
#include <QProcess>

class QComboBox;
class QLabel;
class QProgressBar;
class QPushButton;

namespace wmp {

class EngineClient;

class ImageDialog : public QDialog {
    Q_OBJECT
public:
    ImageDialog(EngineClient *engine, const QString &source, const QString &lastDir, QWidget *parent = nullptr);
    ~ImageDialog() override;

    // A file name for an image of a disc with this label: safe on any system.
    static QString suggestedName(const QString &label, const QString &extension);

signals:
    void folderChosen(const QString &dir);

protected:
    void reject() override;

private:
    void chooseAndStart();
    void start(const QString &out);
    void onOutput();
    void onFinished();
    void loadDrives();

    EngineClient *engine_;
    QString source_, lastDir_, out_;
    QComboBox *drives_;
    QLabel *status_;
    QProgressBar *progress_;
    QPushButton *startBtn_, *cancelBtn_;
    QProcess *proc_ = nullptr;
    QByteArray buffer_;
    bool finished_ = false;
};

} // namespace wmp
