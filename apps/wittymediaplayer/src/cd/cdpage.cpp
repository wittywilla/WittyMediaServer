#include "cdpage.h"

#include "media/player.h"
#include "ui/controlbar.h"
#include "ui/theme.h"
#include "visual/visualizer.h"
#include "engine/engineclient.h"

#include <QAbstractButton>
#include <QApplication>
#include <QCheckBox>
#include <QComboBox>
#include <QCoreApplication>
#include <QDir>
#include <QFileDialog>
#include <QFileInfo>
#include <QGridLayout>
#include <QHBoxLayout>
#include <QHeaderView>
#include <QInputDialog>
#include <QJsonArray>
#include <QJsonDocument>
#include <QLabel>
#include <QLineEdit>
#include <QLinearGradient>
#include <QMouseEvent>
#include <QPainter>
#include <QPainterPath>
#include <QProcess>
#include <QProgressBar>
#include <QPushButton>
#include <QRadialGradient>
#include <QSettings>
#include <QSpinBox>
#include <QStandardPaths>
#include <QStyledItemDelegate>
#include <QTreeWidget>
#include <QVBoxLayout>
#include <cmath>

namespace wmp {

namespace {

// The classic look's own colours: deep blue glass, cool steel, teal gel and amber light.
const QColor kGlassTop(0x24, 0x3a, 0x6e), kGlassBottom(0x0c, 0x13, 0x2c);
const QColor kSteelLight(0xc8, 0xd6, 0xec), kSteelDark(0x4a, 0x5f, 0x8a);
const QColor kGel(0x2f, 0xd6, 0xc0), kGelDeep(0x0b, 0x4f, 0x6a);
const QColor kAmber(0xff, 0xb4, 0x3c), kAmberGhost(0x3a, 0x26, 0x0a), kLcdBack(0x0a, 0x08, 0x05);

enum Column { ColNumber, ColTitle, ColLength, ColCopy };
constexpr int kPercentRole = Qt::UserRole + 1;

QString mmss(double seconds)
{
    const int s = int(std::max(0.0, seconds));
    return QStringLiteral("%1:%2").arg(s / 60).arg(s % 60, 2, 10, QLatin1Char('0'));
}

// Column 3 of the list: a small bar behind the words.
class CopyDelegate : public QStyledItemDelegate {
public:
    using QStyledItemDelegate::QStyledItemDelegate;
    void paint(QPainter *p, const QStyleOptionViewItem &opt, const QModelIndex &index) const override
    {
        QStyledItemDelegate::paint(p, opt, index);
        const QVariant v = index.data(kPercentRole);
        if (!v.isValid())
            return;
        const int pct = v.toInt();
        const QRect r = opt.rect.adjusted(3, opt.rect.height() - 6, -3, -2);
        p->fillRect(r, QColor(0, 0, 0, 90));
        p->fillRect(QRect(r.left(), r.top(), r.width() * pct / 100, r.height()), kGel);
    }
};

} // namespace

// A round gel button with a drawn symbol.
class GelButton : public QAbstractButton {
public:
    enum Glyph { Play, Pause, Stop, Previous, Next };
    GelButton(Glyph glyph, int size, QWidget *parent) : QAbstractButton(parent), glyph_(glyph), size_(size)
    {
        setFixedSize(size, size);
        setFocusPolicy(Qt::StrongFocus);
        setCursor(Qt::PointingHandCursor);
        setAttribute(Qt::WA_Hover);
    }
    void setGlyph(Glyph g)
    {
        glyph_ = g;
        update();
    }
    void setColors(const QColor &light, const QColor &deep, const QColor &ink)
    {
        light_ = light;
        deep_ = deep;
        ink_ = ink;
        update();
    }

protected:
    void paintEvent(QPaintEvent *) override
    {
        QPainter p(this);
        p.setRenderHint(QPainter::Antialiasing);
        const QRectF r = QRectF(rect()).adjusted(1.5, 1.5, -1.5, -1.5);
        QColor light = isEnabled() ? light_ : QColor(0x70, 0x78, 0x88);
        QColor deep = isEnabled() ? deep_ : QColor(0x30, 0x34, 0x40);
        if (underMouse() && isEnabled())
            light = light.lighter(115);
        if (isDown())
            std::swap(light, deep);
        p.setPen(QPen(hasFocus() ? kAmber : QColor(255, 255, 255, 90), hasFocus() ? 2.5 : 1));
        QRadialGradient g(r.center().x(), r.top() + r.height() * 0.2, r.height() * 0.95);
        g.setColorAt(0, light);
        g.setColorAt(1, deep);
        p.setBrush(g);
        p.drawEllipse(r);
        // The shine across the top of the gel.
        QLinearGradient shine(0, r.top(), 0, r.top() + r.height() * 0.5);
        shine.setColorAt(0, QColor(255, 255, 255, 150));
        shine.setColorAt(1, QColor(255, 255, 255, 10));
        p.setPen(Qt::NoPen);
        p.setBrush(shine);
        p.drawEllipse(QRectF(r.left() + r.width() * 0.14, r.top() + 2, r.width() * 0.72, r.height() * 0.46));
        p.setBrush(ink_);
        const double s = size_ * 0.20, cx = r.center().x(), cy = r.center().y();
        switch (glyph_) {
        case Play: {
            const QPointF pts[] = {{cx - s * 0.75, cy - s * 1.15}, {cx + s * 1.25, cy}, {cx - s * 0.75, cy + s * 1.15}};
            p.drawPolygon(pts, 3);
            break;
        }
        case Pause:
            p.drawRoundedRect(QRectF(cx - s, cy - s * 1.1, s * 0.72, s * 2.2), 1.5, 1.5);
            p.drawRoundedRect(QRectF(cx + s * 0.28, cy - s * 1.1, s * 0.72, s * 2.2), 1.5, 1.5);
            break;
        case Stop:
            p.drawRoundedRect(QRectF(cx - s, cy - s, s * 2, s * 2), 2, 2);
            break;
        case Previous: {
            const QPointF pts[] = {{cx + s, cy - s}, {cx - s * 0.4, cy}, {cx + s, cy + s}};
            p.drawPolygon(pts, 3);
            p.drawRect(QRectF(cx - s * 1.05, cy - s, s * 0.4, s * 2));
            break;
        }
        case Next: {
            const QPointF pts[] = {{cx - s, cy - s}, {cx + s * 0.4, cy}, {cx - s, cy + s}};
            p.drawPolygon(pts, 3);
            p.drawRect(QRectF(cx + s * 0.65, cy - s, s * 0.4, s * 2));
            break;
        }
        }
    }

private:
    Glyph glyph_;
    int size_;
    QColor light_ = kGel, deep_ = kGelDeep, ink_ = QColor(0x04, 0x18, 0x22);
};

// The lit number display: seven-segment figures with their unlit ghosts behind them.
class LcdReadout : public QWidget {
public:
    explicit LcdReadout(QWidget *parent) : QWidget(parent)
    {
        setFixedHeight(46);
        setMinimumWidth(190);
        setCursor(Qt::PointingHandCursor);
    }
    void setText(const QString &text)
    {
        if (text != text_) {
            text_ = text;
            update();
        }
    }
    void setColors(const QColor &on, const QColor &ghost, const QColor &back)
    {
        on_ = on;
        ghost_ = ghost;
        back_ = back;
        update();
    }
    std::function<void()> clicked;

protected:
    void mouseReleaseEvent(QMouseEvent *) override
    {
        if (clicked)
            clicked();
    }
    void paintEvent(QPaintEvent *) override
    {
        QPainter p(this);
        p.setRenderHint(QPainter::Antialiasing);
        const QRectF r = QRectF(rect()).adjusted(0.5, 0.5, -0.5, -0.5);
        p.setPen(QPen(QColor(0, 0, 0, 170), 1));
        p.setBrush(back_);
        p.drawRoundedRect(r, 6, 6);
        const double h = height() - 16.0, w = h * 0.52, gap = h * 0.16;
        double total = 0;
        for (QChar c : text_)
            total += c == QLatin1Char(':') ? gap * 2 : (c == QLatin1Char(' ') ? w * 0.7 : w + gap);
        double x = (width() - total) / 2;
        for (QChar c : text_) {
            if (c == QLatin1Char(':')) {
                p.setPen(Qt::NoPen);
                p.setBrush(on_);
                p.drawEllipse(QPointF(x + gap * 0.8, 8 + h * 0.3), 1.8, 1.8);
                p.drawEllipse(QPointF(x + gap * 0.8, 8 + h * 0.7), 1.8, 1.8);
                x += gap * 2;
            } else if (c == QLatin1Char(' ')) {
                x += w * 0.7;
            } else {
                drawDigit(p, x, 8, w, h, c);
                x += w + gap;
            }
        }
    }

private:
    void drawDigit(QPainter &p, double x, double y, double w, double h, QChar c)
    {
        static const QHash<QChar, int> map = {
            {QLatin1Char('0'), 0x3f}, {QLatin1Char('1'), 0x06}, {QLatin1Char('2'), 0x5b}, {QLatin1Char('3'), 0x4f}, {QLatin1Char('4'), 0x66},
            {QLatin1Char('5'), 0x6d}, {QLatin1Char('6'), 0x7d}, {QLatin1Char('7'), 0x07}, {QLatin1Char('8'), 0x7f}, {QLatin1Char('9'), 0x6f},
            {QLatin1Char('-'), 0x40},
        };
        const int lit = map.value(c, 0);
        const double t = h * 0.11;
        const QRectF seg[7] = {
            {x + t, y, w - 2 * t, t},                   // top
            {x + w - t, y + t, t, h / 2 - 1.5 * t},     // upper right
            {x + w - t, y + h / 2 + t / 2, t, h / 2 - 1.5 * t}, // lower right
            {x + t, y + h - t, w - 2 * t, t},           // bottom
            {x, y + h / 2 + t / 2, t, h / 2 - 1.5 * t}, // lower left
            {x, y + t, t, h / 2 - 1.5 * t},             // upper left
            {x + t, y + h / 2 - t / 2, w - 2 * t, t},   // middle
        };
        p.setPen(Qt::NoPen);
        for (int i = 0; i < 7; i++) {
            p.setBrush(lit & (1 << i) ? on_ : ghost_);
            p.drawRoundedRect(seg[i], 1, 1);
        }
    }

    QString text_ = QStringLiteral("-- --:--");
    QColor on_ = kAmber, ghost_ = kAmberGhost, back_ = kLcdBack;
};

// The cover, or a drawn disc when there isn't one.
class CoverView : public QWidget {
public:
    explicit CoverView(QWidget *parent) : QWidget(parent) { setFixedSize(112, 112); }
    void setImage(const QImage &img)
    {
        img_ = img;
        setAccessibleName(img.isNull() ? QStringLiteral("No cover") : QStringLiteral("The album's cover"));
        update();
    }

protected:
    void paintEvent(QPaintEvent *) override
    {
        QPainter p(this);
        p.setRenderHint(QPainter::Antialiasing);
        p.setRenderHint(QPainter::SmoothPixmapTransform);
        const QRectF r = QRectF(rect()).adjusted(1, 1, -1, -1);
        QPainterPath clip;
        clip.addRoundedRect(r, 8, 8);
        p.setClipPath(clip);
        if (!img_.isNull()) {
            p.drawImage(r, img_);
        } else {
            QLinearGradient g(0, 0, 0, height());
            g.setColorAt(0, QColor(0x1a, 0x26, 0x4a));
            g.setColorAt(1, QColor(0x08, 0x0c, 0x1c));
            p.fillRect(r, g);
            const QPointF c = r.center();
            const double R = r.width() * 0.42;
            QConicalGradient sheen(c, 60);
            sheen.setColorAt(0.00, QColor(0xdf, 0xe7, 0xf5));
            sheen.setColorAt(0.20, QColor(0x7f, 0x92, 0xbb));
            sheen.setColorAt(0.35, QColor(0xff, 0xe2, 0xb0));
            sheen.setColorAt(0.50, QColor(0x8f, 0xe8, 0xdc));
            sheen.setColorAt(0.70, QColor(0x6f, 0x82, 0xab));
            sheen.setColorAt(1.00, QColor(0xdf, 0xe7, 0xf5));
            p.setPen(QPen(QColor(255, 255, 255, 80), 1));
            p.setBrush(sheen);
            p.drawEllipse(c, R, R);
            p.setPen(QPen(QColor(255, 255, 255, 120), 1));
            p.setBrush(QColor(0x10, 0x18, 0x30));
            p.drawEllipse(c, R * 0.26, R * 0.26);
            p.setBrush(QColor(0x05, 0x08, 0x12));
            p.drawEllipse(c, R * 0.09, R * 0.09);
        }
        p.setClipping(false);
        p.setPen(QPen(QColor(255, 255, 255, 70), 1));
        p.setBrush(Qt::NoBrush);
        p.drawRoundedRect(r, 8, 8);
    }

private:
    QImage img_;
};

CdPage::CdPage(Player *player, QSettings *settings, QWidget *parent)
    : QWidget(parent), player_(player), settings_(settings), ripper_(new CdRipper(this))
{
    setObjectName(QStringLiteral("cdPage"));
    setAttribute(Qt::WA_StyledBackground, false);
    classic_ = settings_->value(QStringLiteral("cd/classic"), true).toBool();
    build();
    applyLook();

    tick_.setInterval(200);
    connect(&tick_, &QTimer::timeout, this, &CdPage::tick);

    connect(player_, &Player::opened, this, [this] {
        if (!isVisible() || !streamOpen_)
            return;
        if (pendingSeek_ >= 0) {
            if (pendingSeek_ > 0)
                player_->seek(pendingSeek_);
            pendingSeek_ = -1;
        }
    });
    connect(player_, &Player::ended, this, [this] {
        if (!streamOpen_)
            return;
        stopPlaying();
        setStatus(QStringLiteral("The end of the disc."));
    });

    connect(ripper_, &CdRipper::trackStarted, this, [this](int n) {
        ripPercent_ = 0;
        markTrack(n, 0, QStringLiteral("Reading…"));
        tree_->scrollToItem(itemFor(n));
    });
    connect(ripper_, &CdRipper::trackProgress, this, [this](int n, int pct) {
        ripPercent_ = pct;
        markTrack(n, pct, QStringLiteral("%1%").arg(pct));
        overall_->setValue(ripTotal_ > 0 ? (ripDone_ * 100 + pct) / ripTotal_ : 0);
    });
    connect(ripper_, &CdRipper::trackFinished, this, [this](const RipResult &r) {
        results_.push_back(r);
        ripDone_++;
        QTreeWidgetItem *it = itemFor(r.number);
        if (it) {
            it->setData(ColCopy, kPercentRole, QVariant());
            const bool clean = r.ok && r.unreadable == 0;
            it->setText(ColCopy, !r.ok ? QStringLiteral("Not copied") : clean ? (r.verified ? QStringLiteral("Done, checked") : QStringLiteral("Done")) : QStringLiteral("%1 bad sectors").arg(r.unreadable));
            it->setToolTip(ColCopy, r.ok ? QStringLiteral("%1\nCRC %2, %3 retries\n%4").arg(r.quality, r.crc).arg(r.retries).arg(QDir::toNativeSeparators(r.path)) : r.error);
            it->setForeground(ColCopy, clean ? (classic_ ? kGel : Themes::current().ok) : (classic_ ? kAmber : Themes::current().warn));
        }
        overall_->setValue(ripTotal_ > 0 ? ripDone_ * 100 / ripTotal_ : 0);
    });
    connect(ripper_, &CdRipper::finished, this, [this](bool stopped, const QString &message, const QString &report) {
        lastReport_ = report;
        setBusyControls();
        overall_->setVisible(false);
        int good = 0;
        for (const RipResult &r : results_)
            good += r.ok ? 1 : 0;
        for (int i = 0; i < tree_->topLevelItemCount(); i++) {
            QTreeWidgetItem *it = tree_->topLevelItem(i);
            if (it->data(ColCopy, kPercentRole).isValid()) {
                it->setData(ColCopy, kPercentRole, QVariant());
                it->setText(ColCopy, stopped ? QStringLiteral("Stopped") : QString());
            }
        }
        if (stopped)
            setStatus(QStringLiteral("Stopped. %1 of %2 tracks were finished, and the unfinished file was removed.").arg(good).arg(ripTotal_));
        else if (!message.isEmpty())
            setStatus(message);
        else
            setStatus(QStringLiteral("Copied %1 of %2 tracks to %3.").arg(good).arg(ripTotal_).arg(QDir::toNativeSeparators(folder_->text())));
        emit ripFinished();
    });
}

CdPage::~CdPage()
{
    if (imageProc_) {
        imageProc_->disconnect(this);
        imageProc_->closeWriteChannel();
        if (!imageProc_->waitForFinished(5000))
            imageProc_->kill();
    }
}

void CdPage::build()
{
    auto *outer = new QVBoxLayout(this);
    outer->setContentsMargins(14, 10, 14, 10);
    outer->setSpacing(10);

    // The strip along the top.
    auto *head = new QHBoxLayout;
    backBtn_ = new QPushButton(QStringLiteral("Back"), this);
    backBtn_->setAccessibleName(QStringLiteral("Back to the start page"));
    connect(backBtn_, &QPushButton::clicked, this, &CdPage::backRequested);
    head->addWidget(backBtn_);
    headTitle_ = new QLabel(QStringLiteral("CD"), this);
    headTitle_->setObjectName(QStringLiteral("cdHead"));
    head->addWidget(headTitle_, 1);
    auto *lookLabel = new QLabel(QStringLiteral("Look"), this);
    lookLabel->setProperty("role", QStringLiteral("muted"));
    head->addWidget(lookLabel);
    look_ = new QComboBox(this);
    look_->addItems({QStringLiteral("Classic"), QStringLiteral("App theme")});
    look_->setAccessibleName(QStringLiteral("How this screen looks"));
    look_->setCurrentIndex(classic_ ? 0 : 1);
    lookLabel->setBuddy(look_);
    connect(look_, &QComboBox::currentIndexChanged, this, [this](int i) { setClassicLook(i == 0); });
    head->addWidget(look_);
    outer->addLayout(head);

    auto *middle = new QHBoxLayout;
    middle->setSpacing(12);
    outer->addLayout(middle, 1);

    // Now playing.
    auto *nowPanel = new QWidget(this);
    nowPanel->setObjectName(QStringLiteral("cdPanel"));
    nowPanel->setAttribute(Qt::WA_StyledBackground);
    auto *now = new QVBoxLayout(nowPanel);
    now->setContentsMargins(12, 12, 12, 12);
    now->setSpacing(10);
    middle->addWidget(nowPanel, 5);

    visualWidget_ = new QWidget(nowPanel);
    visualHolder_ = new QVBoxLayout(visualWidget_);
    visualHolder_->setContentsMargins(0, 0, 0, 0);
    visual_ = new VisualizerView(player_->audio(), visualWidget_);
    visualHolder_->addWidget(visual_);
    now->addWidget(visualWidget_, 1);

    auto *visualRow = new QHBoxLayout;
    visualName_ = new QLabel(nowPanel);
    visualName_->setProperty("role", QStringLiteral("hint"));
    visualRow->addWidget(visualName_, 1);
    visualBtn_ = new QPushButton(QStringLiteral("Next visual"), nowPanel);
    visualBtn_->setAccessibleName(QStringLiteral("Next visualizer style"));
    connect(visualBtn_, &QPushButton::clicked, visual_, &VisualizerView::nextStyle);
    visualRow->addWidget(visualBtn_);
    now->addLayout(visualRow);
    const auto showStyle = [this](int s) {
        visualName_->setText(VisualizerView::styleNames().value(s));
        settings_->setValue(QStringLiteral("cd/visual"), s);
    };
    connect(visual_, &VisualizerView::styleChanged, this, showStyle);
    visual_->setStyle(settings_->value(QStringLiteral("cd/visual"), 0).toInt());

    auto *infoRow = new QHBoxLayout;
    infoRow->setSpacing(12);
    coverView_ = new CoverView(nowPanel);
    coverView_->setImage({});
    infoRow->addWidget(coverView_);
    auto *infoCol = new QVBoxLayout;
    infoCol->setSpacing(4);
    nowTitle_ = new QLabel(QStringLiteral("Press play"), nowPanel);
    nowTitle_->setObjectName(QStringLiteral("cdNowTitle"));
    nowTitle_->setWordWrap(true);
    nowTitle_->setMinimumWidth(100);
    nowArtist_ = new QLabel(nowPanel);
    nowArtist_->setObjectName(QStringLiteral("cdNowArtist"));
    nowArtist_->setWordWrap(true);
    infoCol->addWidget(nowTitle_);
    infoCol->addWidget(nowArtist_);
    infoCol->addStretch(1);
    lcd_ = new LcdReadout(nowPanel);
    lcd_->setAccessibleName(QStringLiteral("Track and time. Click to switch between time gone and time left."));
    lcd_->clicked = [this] {
        showRemaining_ = !showRemaining_;
        tick();
    };
    infoCol->addWidget(lcd_);
    infoRow->addLayout(infoCol, 1);
    now->addLayout(infoRow);

    seek_ = new SeekBar(nowPanel);
    seek_->setAccessibleName(QStringLiteral("Position in this track"));
    connect(seek_, &SeekBar::seekRequested, this, [this](double s) {
        if (streamOpen_ && current_ < info_.tracks.size())
            player_->seek(info_.startOf(current_) + s);
    });
    now->addWidget(seek_);

    auto *transport = new QHBoxLayout;
    transport->setSpacing(8);
    prevBtn_ = new GelButton(GelButton::Previous, 40, nowPanel);
    prevBtn_->setAccessibleName(QStringLiteral("Previous track"));
    prevBtn_->setToolTip(QStringLiteral("Previous track (P)"));
    playBtn_ = new GelButton(GelButton::Play, 58, nowPanel);
    playBtn_->setAccessibleName(QStringLiteral("Play"));
    playBtn_->setToolTip(QStringLiteral("Play or pause (Space)"));
    stopBtn_ = new GelButton(GelButton::Stop, 40, nowPanel);
    stopBtn_->setAccessibleName(QStringLiteral("Stop"));
    stopBtn_->setToolTip(QStringLiteral("Stop (S)"));
    nextBtn_ = new GelButton(GelButton::Next, 40, nowPanel);
    nextBtn_->setAccessibleName(QStringLiteral("Next track"));
    nextBtn_->setToolTip(QStringLiteral("Next track (N)"));
    connect(prevBtn_, &QAbstractButton::clicked, this, &CdPage::previous);
    connect(playBtn_, &QAbstractButton::clicked, this, &CdPage::togglePlay);
    connect(stopBtn_, &QAbstractButton::clicked, this, &CdPage::stopPlaying);
    connect(nextBtn_, &QAbstractButton::clicked, this, &CdPage::next);
    transport->addWidget(prevBtn_);
    transport->addWidget(playBtn_);
    transport->addWidget(stopBtn_);
    transport->addWidget(nextBtn_);
    transport->addStretch(1);
    auto *volLabel = new QLabel(QStringLiteral("Volume"), nowPanel);
    volLabel->setProperty("role", QStringLiteral("hint"));
    transport->addWidget(volLabel);
    volume_ = new VolumeBar(nowPanel);
    volume_->setAccessibleName(QStringLiteral("Volume"));
    volume_->setFixedWidth(130);
    connect(volume_, &QAbstractSlider::valueChanged, this, &CdPage::volumeChanged);
    transport->addWidget(volume_);
    now->addLayout(transport);

    // The tracks.
    auto *listPanel = new QWidget(this);
    listPanel->setObjectName(QStringLiteral("cdPanel"));
    listPanel->setAttribute(Qt::WA_StyledBackground);
    auto *list = new QVBoxLayout(listPanel);
    list->setContentsMargins(12, 12, 12, 12);
    list->setSpacing(8);
    middle->addWidget(listPanel, 6);

    auto *names = new QGridLayout;
    names->setHorizontalSpacing(8);
    names->setVerticalSpacing(6);
    const auto addEdit = [&](int row, int col, const QString &label, QLineEdit **out, int span = 1) {
        auto *l = new QLabel(label, listPanel);
        l->setProperty("role", QStringLiteral("hint"));
        *out = new QLineEdit(listPanel);
        (*out)->setAccessibleName(label);
        l->setBuddy(*out);
        names->addWidget(l, row, col);
        names->addWidget(*out, row, col + 1, 1, span);
    };
    addEdit(0, 0, QStringLiteral("Album"), &album_, 3);
    addEdit(1, 0, QStringLiteral("Artist"), &artist_);
    addEdit(1, 2, QStringLiteral("Year"), &year_);
    year_->setMaximumWidth(90);
    list->addLayout(names);
    connect(album_, &QLineEdit::textEdited, this, [this] { showTrack(); });
    connect(artist_, &QLineEdit::textEdited, this, [this] { showTrack(); });

    tree_ = new QTreeWidget(listPanel);
    tree_->setObjectName(QStringLiteral("cdTracks"));
    tree_->setAccessibleName(QStringLiteral("Tracks. Press Enter to play one, and tick the ones to copy."));
    tree_->setColumnCount(4);
    tree_->setHeaderLabels({QStringLiteral("#"), QStringLiteral("Title"), QStringLiteral("Time"), QStringLiteral("Copy")});
    tree_->setRootIsDecorated(false);
    tree_->setUniformRowHeights(true);
    tree_->setAlternatingRowColors(true);
    tree_->setSelectionMode(QAbstractItemView::SingleSelection);
    tree_->setEditTriggers(QAbstractItemView::EditKeyPressed);
    tree_->header()->setStretchLastSection(false);
    tree_->header()->setSectionResizeMode(ColTitle, QHeaderView::Stretch);
    tree_->header()->setSectionResizeMode(ColNumber, QHeaderView::Fixed);
    tree_->header()->setSectionResizeMode(ColLength, QHeaderView::Fixed);
    tree_->header()->setSectionResizeMode(ColCopy, QHeaderView::Fixed);
    tree_->setColumnWidth(ColNumber, 72);
    tree_->setColumnWidth(ColLength, 60);
    tree_->setColumnWidth(ColCopy, 120);
    tree_->setItemDelegateForColumn(ColCopy, new CopyDelegate(tree_));
    connect(tree_, &QTreeWidget::itemActivated, this, [this](QTreeWidgetItem *it, int) { playTrack(tree_->indexOfTopLevelItem(it)); });
    list->addWidget(tree_, 1);

    auto *listRow = new QHBoxLayout;
    auto *all = new QPushButton(QStringLiteral("All"), listPanel);
    all->setAccessibleName(QStringLiteral("Tick every track"));
    auto *none = new QPushButton(QStringLiteral("None"), listPanel);
    none->setAccessibleName(QStringLiteral("Untick every track"));
    const auto tickAll = [this](Qt::CheckState s) {
        for (int i = 0; i < tree_->topLevelItemCount(); i++)
            tree_->topLevelItem(i)->setCheckState(ColNumber, s);
    };
    connect(all, &QPushButton::clicked, this, [tickAll] { tickAll(Qt::Checked); });
    connect(none, &QPushButton::clicked, this, [tickAll] { tickAll(Qt::Unchecked); });
    lookupBtn_ = new QPushButton(QStringLiteral("Look up names"), listPanel);
    lookupBtn_->setToolTip(QStringLiteral("Asks MusicBrainz what this disc is. Only the disc's track layout is sent, and only when you press this."));
    connect(lookupBtn_, &QPushButton::clicked, this, &CdPage::lookUp);
    listRow->addWidget(all);
    listRow->addWidget(none);
    listRow->addStretch(1);
    listRow->addWidget(lookupBtn_);
    list->addLayout(listRow);

    // Copying.
    auto *ripPanel = new QWidget(this);
    ripPanel->setObjectName(QStringLiteral("cdPanel"));
    ripPanel->setAttribute(Qt::WA_StyledBackground);
    auto *rip = new QGridLayout(ripPanel);
    rip->setContentsMargins(12, 10, 12, 10);
    rip->setHorizontalSpacing(10);
    rip->setVerticalSpacing(6);
    outer->addWidget(ripPanel);
    const auto hint = [&](const QString &text, QWidget *buddy) {
        auto *l = new QLabel(text, ripPanel);
        l->setProperty("role", QStringLiteral("hint"));
        l->setBuddy(buddy);
        return l;
    };
    format_ = new QComboBox(ripPanel);
    format_->setAccessibleName(QStringLiteral("Format"));
    format_->setToolTip(QStringLiteral("Written by FFmpeg (LGPL). MP3 uses LAME (LGPL), and Opus and Vorbis use Xiph.Org's libraries (BSD). See Help, About."));
    for (const RipFormat &f : ripFormats())
        format_->addItem(f.name, f.id);
    quality_ = new QComboBox(ripPanel);
    quality_->setAccessibleName(QStringLiteral("Quality"));
    connect(format_, &QComboBox::currentIndexChanged, this, &CdPage::refillQualities);
    folder_ = new QLineEdit(ripPanel);
    folder_->setAccessibleName(QStringLiteral("Folder to copy to"));
    chooseBtn_ = new QPushButton(QStringLiteral("Choose…"), ripPanel);
    chooseBtn_->setAccessibleName(QStringLiteral("Choose the folder to copy to"));
    connect(chooseBtn_, &QPushButton::clicked, this, &CdPage::chooseFolder);
    template_ = new QLineEdit(ripPanel);
    template_->setAccessibleName(QStringLiteral("How files are named"));
    template_->setToolTip(QStringLiteral("You can use {albumartist} {artist} {album} {title} {track} {tracks} {disc} {year}. A slash makes a folder."));
    secure_ = new QCheckBox(QStringLiteral("Check every read twice"), ripPanel);
    secure_->setToolTip(QStringLiteral("Reads each track twice and again wherever the reads disagree. Slower, and how to be sure of a scratched disc."));
    offset_ = new QSpinBox(ripPanel);
    offset_->setRange(-5000, 5000);
    offset_->setAccessibleName(QStringLiteral("Drive read offset, in samples"));
    offset_->setToolTip(QStringLiteral("Most drives read a few samples early or late. Leave it at 0 unless you know your drive's figure."));
    ripBtn_ = new QPushButton(QStringLiteral("Copy ticked tracks"), ripPanel);
    ripBtn_->setProperty("role", QStringLiteral("primary"));
    connect(ripBtn_, &QPushButton::clicked, this, &CdPage::startRip);
    stopRipBtn_ = new QPushButton(QStringLiteral("Stop copying"), ripPanel);
    connect(stopRipBtn_, &QPushButton::clicked, this, &CdPage::stopRip);
    imageBtn_ = new QPushButton(QStringLiteral("Make image"), ripPanel);
    imageBtn_->setToolTip(QStringLiteral("Copies the disc's audio to a .cue and .bin, which this player and WittyMediaServer can read."));
    connect(imageBtn_, &QPushButton::clicked, this, &CdPage::chooseImage);

    rip->addWidget(hint(QStringLiteral("Format"), format_), 0, 0);
    rip->addWidget(format_, 0, 1);
    rip->addWidget(hint(QStringLiteral("Quality"), quality_), 0, 2);
    rip->addWidget(quality_, 0, 3);
    rip->addWidget(hint(QStringLiteral("Folder"), folder_), 0, 4);
    rip->addWidget(folder_, 0, 5);
    rip->addWidget(chooseBtn_, 0, 6);
    rip->addWidget(hint(QStringLiteral("Names"), template_), 1, 0);
    rip->addWidget(template_, 1, 1, 1, 3);
    auto *checks = new QHBoxLayout;
    checks->addWidget(secure_);
    checks->addSpacing(14);
    offsetLabel_ = hint(QStringLiteral("Drive offset"), offset_);
    checks->addWidget(offsetLabel_);
    checks->addWidget(offset_);
    checks->addStretch(1);
    rip->addLayout(checks, 1, 4, 1, 3);
    rip->setColumnStretch(1, 2);
    rip->setColumnStretch(3, 2);
    rip->setColumnStretch(5, 3);

    auto *bottom = new QHBoxLayout;
    statusLabel_ = new QLabel(ripPanel);
    statusLabel_->setObjectName(QStringLiteral("cdStatus"));
    statusLabel_->setWordWrap(true);
    bottom->addWidget(statusLabel_, 1);
    overall_ = new QProgressBar(ripPanel);
    overall_->setRange(0, 100);
    overall_->setFixedWidth(180);
    overall_->setAccessibleName(QStringLiteral("All the tracks being copied"));
    overall_->hide();
    bottom->addWidget(overall_);
    bottom->addWidget(imageBtn_);
    bottom->addWidget(stopRipBtn_);
    bottom->addWidget(ripBtn_);
    rip->addLayout(bottom, 2, 0, 1, 7);

    // What was used last time.
    const int fmt = format_->findData(settings_->value(QStringLiteral("cd/format"), QStringLiteral("flac")).toString());
    format_->setCurrentIndex(qMax(0, fmt));
    refillQualities();
    folder_->setText(settings_->value(QStringLiteral("cd/folder"), QStandardPaths::writableLocation(QStandardPaths::MusicLocation)).toString());
    template_->setText(settings_->value(QStringLiteral("cd/template"), defaultRipTemplate()).toString());
    secure_->setChecked(settings_->value(QStringLiteral("cd/secure"), true).toBool());
    offset_->setValue(settings_->value(QStringLiteral("cd/offset"), 0).toInt());
    setBusyControls();
}

void CdPage::refillQualities()
{
    const QString id = format_->currentData().toString();
    const RipFormat *f = ripFormat(id);
    quality_->clear();
    if (!f)
        return;
    for (const RipQuality &q : f->qualities)
        quality_->addItem(q.label, q.id);
    const QString last = settings_->value(QStringLiteral("cd/quality-%1").arg(id)).toString();
    const int at = quality_->findData(last);
    quality_->setCurrentIndex(at >= 0 && !last.isEmpty() ? at : f->defaultQuality);
    quality_->setEnabled(f->qualities.size() > 1);
}

QStringList CdPage::qualitiesOf(const QString &format) const
{
    QStringList out;
    if (const RipFormat *f = ripFormat(format))
        for (const RipQuality &q : f->qualities)
            out << q.id;
    return out;
}

void CdPage::paintEvent(QPaintEvent *)
{
    QPainter p(this);
    if (!classic_) {
        p.fillRect(rect(), Themes::current().bg);
        return;
    }
    QLinearGradient g(0, 0, 0, height());
    g.setColorAt(0, kGlassTop);
    g.setColorAt(1, kGlassBottom);
    p.fillRect(rect(), g);
    // A sweep of light across the glass, in the manner of the desktops of the time.
    p.setRenderHint(QPainter::Antialiasing);
    QPainterPath sweep;
    sweep.moveTo(0, 0);
    sweep.lineTo(width(), 0);
    sweep.lineTo(width(), height() * 0.06);
    sweep.cubicTo(width() * 0.7, height() * 0.22, width() * 0.3, height() * 0.02, 0, height() * 0.16);
    sweep.closeSubpath();
    QLinearGradient light(0, 0, 0, height() * 0.2);
    light.setColorAt(0, QColor(255, 255, 255, 46));
    light.setColorAt(1, QColor(255, 255, 255, 4));
    p.fillPath(sweep, light);
}

void CdPage::applyLook()
{
    const Theme &t = Themes::current();
    if (classic_) {
        setStyleSheet(QStringLiteral(R"(
QWidget#cdPanel { background: qlineargradient(x1:0,y1:0,x2:0,y2:1, stop:0 rgba(70,96,150,120), stop:0.5 rgba(26,38,72,190), stop:1 rgba(12,18,38,220));
  border: 1px solid rgba(200,214,236,70); border-top: 1px solid rgba(232,240,255,150); border-radius: 12px; }
QLabel { color: #dbe6f7; background: transparent; }
QLabel[role="hint"] { color: #9fb2d3; font-size: 12px; }
QLabel[role="muted"] { color: #9fb2d3; }
QLabel#cdHead { color: #f2f6ff; font-size: 19px; font-weight: 700; }
QLabel#cdNowTitle { color: #ffffff; font-size: 20px; font-weight: 650; }
QLabel#cdNowArtist { color: #a8e9df; font-size: 14px; }
QLabel#cdStatus { color: #ffd38a; font-size: 13px; }
QPushButton { color: #eaf1ff; border: 1px solid #6c83b0; border-top-color: #c8d6ec; border-radius: 9px; padding: 6px 14px;
  background: qlineargradient(x1:0,y1:0,x2:0,y2:1, stop:0 #62799f, stop:0.48 #41557e, stop:0.52 #2f4167, stop:1 #3b4f77); }
QPushButton:hover { background: qlineargradient(x1:0,y1:0,x2:0,y2:1, stop:0 #7a93bc, stop:0.48 #526a96, stop:0.52 #3c5280, stop:1 #4a6190); }
QPushButton:pressed { background: #2a3a5e; }
QPushButton:focus { border: 2px solid #ffb43c; padding: 5px 13px; }
QPushButton:disabled { color: #76849e; border-color: #3b4763; background: #252f49; }
QPushButton[role="primary"] { color: #04202a; font-weight: 700; border-color: #2fd6c0; border-top-color: #c7fff5;
  background: qlineargradient(x1:0,y1:0,x2:0,y2:1, stop:0 #a5f5e8, stop:0.48 #41dcc7, stop:0.52 #1fb7a6, stop:1 #2ccbb8); }
QPushButton[role="primary"]:hover { background: qlineargradient(x1:0,y1:0,x2:0,y2:1, stop:0 #c5fbf2, stop:0.48 #64e8d6, stop:0.52 #2ccbb8, stop:1 #41dcc7); }
QPushButton[role="primary"]:disabled { color: #76849e; border-color: #3b4763; background: #252f49; }
QLineEdit, QSpinBox, QComboBox { color: #f0f5ff; background: #0a1024; border: 1px solid #5a6f9b; border-top-color: #2a3657; border-radius: 6px; padding: 4px 7px; selection-background-color: #1fb7a6; }
QLineEdit:focus, QSpinBox:focus, QComboBox:focus { border: 2px solid #ffb43c; padding: 3px 6px; }
QComboBox QAbstractItemView { color: #f0f5ff; background: #101a38; selection-background-color: #1fb7a6; selection-color: #04202a; border: 1px solid #5a6f9b; }
QCheckBox { color: #dbe6f7; background: transparent; spacing: 7px; }
QCheckBox::indicator { width: 15px; height: 15px; border: 1px solid #7f96c2; border-radius: 3px; background: #0a1024; }
QCheckBox::indicator:checked { background: #2fd6c0; border-color: #a5f5e8; }
QCheckBox:focus { color: #ffd38a; }
QCheckBox:disabled { color: #76849e; }
QCheckBox::indicator:disabled { background: #252f49; border-color: #3b4763; }
QSpinBox:disabled, QLineEdit:disabled, QComboBox:disabled { color: #76849e; background: #151c33; border-color: #3b4763; }
QTreeWidget { color: #e6eeff; background: #070c1c; alternate-background-color: #0d1530; border: 1px solid #3f5078; border-radius: 8px; outline: none; }
QTreeWidget:focus { border: 2px solid #ffb43c; }
QTreeWidget::item { padding: 3px 2px; }
QTreeWidget::item:selected { color: #04202a; background: qlineargradient(x1:0,y1:0,x2:0,y2:1, stop:0 #a5f5e8, stop:1 #2ccbb8); }
QTreeWidget::indicator { width: 14px; height: 14px; border: 1px solid #7f96c2; border-radius: 3px; background: #0a1024; }
QTreeWidget::indicator:checked { background: #2fd6c0; border-color: #a5f5e8; }
QHeaderView::section { color: #cfdcf3; border: none; border-right: 1px solid #1d2947; padding: 4px 6px;
  background: qlineargradient(x1:0,y1:0,x2:0,y2:1, stop:0 #4a5f8a, stop:1 #232f50); }
QScrollBar:vertical { background: #0a1024; width: 13px; border-radius: 6px; }
QScrollBar::handle:vertical { min-height: 24px; border-radius: 6px; background: qlineargradient(x1:0,y1:0,x2:1,y2:0, stop:0 #8ea4cd, stop:1 #4a5f8a); }
QScrollBar::add-line:vertical, QScrollBar::sub-line:vertical { height: 0; }
QProgressBar { color: #f0f5ff; background: #0a1024; border: 1px solid #5a6f9b; border-radius: 6px; text-align: center; }
QProgressBar::chunk { border-radius: 5px; background: qlineargradient(x1:0,y1:0,x2:0,y2:1, stop:0 #a5f5e8, stop:1 #1fb7a6); }
)"));
        visual_->setColors(kGel, kAmber, QColor(0x05, 0x08, 0x12));
        lcd_->setColors(kAmber, kAmberGhost, kLcdBack);
        for (GelButton *b : {prevBtn_, playBtn_, stopBtn_, nextBtn_})
            b->setColors(kGel, kGelDeep, QColor(0x04, 0x18, 0x22));
    } else {
        setStyleSheet(QStringLiteral(
            "QWidget#cdPanel { background: %1; border: 1px solid %2; border-radius: %3px; }"
            "QLabel { background: transparent; }"
            "QLabel#cdHead { font-size: 18px; font-weight: 600; }"
            "QLabel#cdNowTitle { font-size: 19px; font-weight: 600; }"
            "QLabel#cdNowArtist { color: %4; }"
            "QLabel#cdStatus { color: %4; }"
            "QComboBox, QSpinBox { color: %5; background: %6; border: 1px solid %7; border-radius: 6px; padding: 4px 7px; }"
            "QComboBox:focus, QSpinBox:focus { border: 2px solid %8; padding: 3px 6px; }"
            "QComboBox:disabled, QSpinBox:disabled { color: %9; }"
            "QComboBox QAbstractItemView { color: %5; background: %6; selection-background-color: %10; selection-color: %11; }"
            "QTreeWidget { color: %5; background: %12; alternate-background-color: %1; border: 1px solid %7; border-radius: 6px; outline: none; }"
            "QTreeWidget:focus { border: 2px solid %8; }"
            "QTreeWidget::item:selected { color: %11; background: %10; }"
            "QHeaderView::section { color: %4; background: %1; border: none; padding: 4px 6px; }")
                          .arg(t.bg2.name(), t.bg4.name())
                          .arg(t.radius)
                          .arg(t.fg2.name(), t.fg.name(), t.bg3.name(), t.controlLine.name(), t.focus.name(), t.fg3.name())
                          .arg(t.accent.name(), t.accentFg.name(), t.bg.name()));
        visual_->setColors(t.accent, t.accentText, t.bg);
        lcd_->setColors(t.accentText, t.bg3, t.bg);
        for (GelButton *b : {prevBtn_, playBtn_, stopBtn_, nextBtn_})
            b->setColors(t.accentHover, t.accent, t.accentFg);
    }
    update();
}

void CdPage::setClassicLook(bool on)
{
    classic_ = on;
    settings_->setValue(QStringLiteral("cd/classic"), on);
    if (look_->currentIndex() != (on ? 0 : 1))
        look_->setCurrentIndex(on ? 0 : 1);
    applyLook();
}

void CdPage::setVolumePercent(int percent)
{
    const QSignalBlocker block(volume_);
    volume_->setValue(percent);
}

void CdPage::setVisualizer(QWidget *visualizer)
{
    while (QLayoutItem *it = visualHolder_->takeAt(0)) {
        if (it->widget())
            it->widget()->hide();
        delete it;
    }
    visualHolder_->addWidget(visualizer);
    visualizer->show();
    visualBtn_->setVisible(visualizer == visual_);
    visualName_->setVisible(visualizer == visual_);
}

void CdPage::setDisc(const CdInfo &info)
{
    stopPlaying();
    info_ = info;
    current_ = 0;
    results_.clear();
    lastReport_.clear();
    cover_.clear();
    coverView_->setImage({});
    album_->setText(info.title);
    artist_->setText(info.performer);
    year_->setText(info.date.left(4));
    headTitle_->setText(info.drive ? QStringLiteral("Audio CD in %1").arg(info.path.left(2)) : QFileInfo(info.path).fileName());
    fillTracks();
    // An image reads the same every time: these only mean something for a drive.
    secure_->setVisible(info.drive);
    offset_->setVisible(info.drive);
    offsetLabel_->setVisible(info.drive);
    showTrack();
    setBusyControls();
    setStatus(info.warnings.isEmpty() ? QStringLiteral("%1 tracks.").arg(info.tracks.size()) : info.warnings.join(QLatin1Char(' ')));
    tick_.start();
}

void CdPage::fillTracks()
{
    tree_->clear();
    for (const CdTrack &t : info_.tracks) {
        auto *it = new QTreeWidgetItem(tree_);
        it->setFlags(it->flags() | Qt::ItemIsUserCheckable | Qt::ItemIsEditable);
        it->setCheckState(ColNumber, Qt::Checked);
        it->setText(ColNumber, QStringLiteral("%1").arg(t.number, 2, 10, QLatin1Char('0')));
        it->setText(ColTitle, t.title.isEmpty() ? QStringLiteral("Track %1").arg(t.number) : t.title);
        it->setText(ColLength, mmss(t.seconds()));
        it->setTextAlignment(ColLength, Qt::AlignRight | Qt::AlignVCenter);
        it->setData(ColNumber, Qt::UserRole, t.number);
        if (t.preEmphasis)
            it->setToolTip(ColTitle, QStringLiteral("This track was mastered with pre-emphasis. Copies keep the disc's samples as they are."));
    }
    if (tree_->topLevelItemCount() > 0)
        tree_->setCurrentItem(tree_->topLevelItem(0));
}

void CdPage::closeDisc()
{
    stopPlaying();
    tick_.stop();
    stopRip();
    if (imageProc_)
        imageProc_->closeWriteChannel();
    if (lookup_)
        lookup_->kill();
}

bool CdPage::handleKey(int key, QWidget *focus)
{
    const bool typing = qobject_cast<QLineEdit *>(focus) || qobject_cast<QComboBox *>(focus) || qobject_cast<QSpinBox *>(focus);
    if (typing)
        return false;
    const bool onSlider = qobject_cast<QAbstractSlider *>(focus);
    const bool onButton = qobject_cast<QAbstractButton *>(focus);
    const bool onList = focus == tree_ || (focus && tree_->isAncestorOf(focus));
    switch (key) {
    case Qt::Key_Space:
        // Space ticks a track in the list, and presses a focused button.
        if (onButton || onList)
            return false;
        togglePlay();
        return true;
    case Qt::Key_K:
        togglePlay();
        return true;
    case Qt::Key_N:
        next();
        return true;
    case Qt::Key_P:
        previous();
        return true;
    case Qt::Key_S:
        stopPlaying();
        return true;
    case Qt::Key_V:
        visual_->nextStyle();
        return true;
    case Qt::Key_Left:
    case Qt::Key_Right:
        if (onSlider || onList)
            return false;
        if (streamOpen_)
            player_->seek(qMax(0.0, player_->position() + (key == Qt::Key_Left ? -10 : 10)));
        return true;
    case Qt::Key_Up:
    case Qt::Key_Down:
        if (onSlider || onList)
            return false;
        volume_->setValue(qBound(0, volume_->value() + (key == Qt::Key_Up ? 5 : -5), 100));
        return true;
    default:
        return false;
    }
}

void CdPage::playTrack(int index)
{
    if (index < 0 || index >= info_.tracks.size())
        return;
    if (ripper_->running() && info_.drive) {
        setStatus(QStringLiteral("The drive can't copy and play at once. Stop copying first, or wait until it's done."));
        return;
    }
    current_ = index;
    const double at = info_.startOf(index);
    seekTarget_ = at;
    seekWait_ = 0;
    player_->setPaused(false);
    if (streamOpen_ && player_->hasMedia()) {
        player_->seek(at);
    } else {
        MediaSpec spec;
        spec.filePath = info_.path;
        spec.cd = true;
        pendingSeek_ = at;
        streamOpen_ = true;
        player_->open(spec);
    }
    playing_ = true;
    userPaused_ = false;
    playBtn_->setGlyph(GelButton::Pause);
    playBtn_->setAccessibleName(QStringLiteral("Pause"));
    showTrack();
    tick();
}

void CdPage::togglePlay()
{
    if (!streamOpen_) {
        const auto *sel = tree_->currentItem();
        playTrack(sel ? tree_->indexOfTopLevelItem(const_cast<QTreeWidgetItem *>(sel)) : 0);
        return;
    }
    userPaused_ = !userPaused_;
    playing_ = !userPaused_;
    player_->setPaused(userPaused_);
    playBtn_->setGlyph(userPaused_ ? GelButton::Play : GelButton::Pause);
    playBtn_->setAccessibleName(userPaused_ ? QStringLiteral("Play") : QStringLiteral("Pause"));
}

void CdPage::stopPlaying()
{
    if (streamOpen_)
        player_->stop();
    streamOpen_ = false;
    playing_ = false;
    userPaused_ = false;
    pendingSeek_ = -1;
    seekTarget_ = -1;
    playBtn_->setGlyph(GelButton::Play);
    playBtn_->setAccessibleName(QStringLiteral("Play"));
    seek_->setPosition(0);
    showTrack();
    tick();
}

void CdPage::previous()
{
    if (info_.tracks.isEmpty())
        return;
    // Part way through a track, this means its start.
    if (streamOpen_ && player_->position() - info_.startOf(current_) > 3) {
        playTrack(current_);
        return;
    }
    const int to = qMax(0, current_ - 1);
    if (streamOpen_)
        playTrack(to);
    else {
        current_ = to;
        showTrack();
        tick();
    }
}

void CdPage::next()
{
    if (current_ + 1 >= info_.tracks.size())
        return;
    if (streamOpen_)
        playTrack(current_ + 1);
    else {
        current_++;
        showTrack();
        tick();
    }
}

void CdPage::showTrack()
{
    const bool has = current_ >= 0 && current_ < info_.tracks.size();
    for (int i = 0; i < tree_->topLevelItemCount(); i++) {
        QTreeWidgetItem *it = tree_->topLevelItem(i);
        QFont f = it->font(ColTitle);
        f.setBold(i == current_ && streamOpen_);
        it->setFont(ColTitle, f);
    }
    if (!has) {
        nowTitle_->setText(QStringLiteral("No disc"));
        nowArtist_->clear();
        return;
    }
    QTreeWidgetItem *it = tree_->topLevelItem(current_);
    nowTitle_->setText(it ? it->text(ColTitle) : QString());
    if (it && streamOpen_ && tree_->currentItem() != it && !qobject_cast<QLineEdit *>(QApplication::focusWidget()))
        tree_->setCurrentItem(it);
    const CdTrack &t = info_.tracks[current_];
    const QString artist = t.performer.isEmpty() ? artist_->text() : t.performer;
    const QString album = album_->text();
    nowArtist_->setText(artist.isEmpty() ? album : album.isEmpty() ? artist : QStringLiteral("%1, %2").arg(artist, album));
    seek_->setDuration(t.seconds());
}

void CdPage::tick()
{
    if (info_.tracks.isEmpty())
        return;
    double within = 0;
    if (streamOpen_ && player_->hasMedia()) {
        const double pos = player_->position();
        if (seekTarget_ >= 0 && (qAbs(pos - seekTarget_) < 1.5 || ++seekWait_ > 25))
            seekTarget_ = -1;
        // Playing on into the next track, or seeking there, moves the display with it.
        if (seekTarget_ < 0 && pos >= 0) {
            int at = 0;
            for (int i = 0; i < info_.tracks.size(); i++)
                if (pos + 0.05 >= info_.startOf(i))
                    at = i;
            if (at != current_) {
                current_ = at;
                showTrack();
            }
        }
        within = qMax(0.0, pos - info_.startOf(current_));
        if (!seek_->dragging())
            seek_->setPosition(within);
    }
    const double len = info_.tracks[current_].seconds();
    const double shown = showRemaining_ ? qMax(0.0, len - within) : within;
    lcd_->setText(QStringLiteral("%1 %2%3:%4")
                      .arg(info_.tracks[current_].number, 2, 10, QLatin1Char('0'))
                      .arg(showRemaining_ ? QStringLiteral("-") : QString())
                      .arg(int(shown) / 60, 2, 10, QLatin1Char('0'))
                      .arg(int(shown) % 60, 2, 10, QLatin1Char('0')));
}

QString CdPage::statusText() const
{
    return statusLabel_->text();
}

void CdPage::setStatus(const QString &text)
{
    statusLabel_->setText(text);
    emit statusChanged(text);
}

bool CdPage::busy() const
{
    return ripper_->running() || imageProc_;
}

void CdPage::setBusyControls()
{
    const bool b = busy();
    ripBtn_->setVisible(!b);
    imageBtn_->setVisible(!b);
    stopRipBtn_->setVisible(b);
    ripBtn_->setEnabled(!info_.tracks.isEmpty());
    imageBtn_->setEnabled(!info_.tracks.isEmpty());
    format_->setEnabled(!b);
    const RipFormat *f = ripFormat(format_->currentData().toString());
    quality_->setEnabled(!b && f && f->qualities.size() > 1);
    folder_->setEnabled(!b);
    chooseBtn_->setEnabled(!b);
    template_->setEnabled(!b);
    secure_->setEnabled(!b && info_.drive);
    offset_->setEnabled(!b && info_.drive);
    backBtn_->setEnabled(!b);
    lookupBtn_->setEnabled(!b && !lookup_ && !info_.discId.isEmpty());
}

void CdPage::setChecked(const QVector<int> &numbers)
{
    for (int i = 0; i < tree_->topLevelItemCount(); i++) {
        QTreeWidgetItem *it = tree_->topLevelItem(i);
        it->setCheckState(ColNumber, numbers.contains(it->data(ColNumber, Qt::UserRole).toInt()) ? Qt::Checked : Qt::Unchecked);
    }
}

QVector<int> CdPage::checkedTracks() const
{
    QVector<int> out;
    for (int i = 0; i < tree_->topLevelItemCount(); i++) {
        const QTreeWidgetItem *it = tree_->topLevelItem(i);
        if (it->checkState(ColNumber) == Qt::Checked)
            out << it->data(ColNumber, Qt::UserRole).toInt();
    }
    return out;
}

void CdPage::setRipOptions(const QString &format, const QString &quality, const QString &folder, const QString &nameTemplate, bool secure, int offset)
{
    format_->setCurrentIndex(format_->findData(format));
    const int q = quality_->findData(quality);
    if (q >= 0)
        quality_->setCurrentIndex(q);
    folder_->setText(folder);
    if (!nameTemplate.isEmpty())
        template_->setText(nameTemplate);
    secure_->setChecked(secure);
    offset_->setValue(offset);
}

void CdPage::saveOptions()
{
    settings_->setValue(QStringLiteral("cd/format"), format_->currentData());
    settings_->setValue(QStringLiteral("cd/quality-%1").arg(format_->currentData().toString()), quality_->currentData());
    settings_->setValue(QStringLiteral("cd/folder"), folder_->text());
    settings_->setValue(QStringLiteral("cd/template"), template_->text());
    settings_->setValue(QStringLiteral("cd/secure"), secure_->isChecked());
    settings_->setValue(QStringLiteral("cd/offset"), offset_->value());
}

RipJob CdPage::buildJob() const
{
    RipJob job;
    job.source = info_.path;
    job.format = format_->currentData().toString();
    job.quality = quality_->currentData().toString();
    job.folder = folder_->text().trimmed();
    job.nameTemplate = template_->text().trimmed();
    job.secure = secure_->isChecked() && info_.drive;
    job.offset = info_.drive ? offset_->value() : 0;
    job.album = album_->text().trimmed();
    const QVector<int> wanted = checkedTracks();
    for (int i = 0; i < info_.tracks.size(); i++) {
        const CdTrack &t = info_.tracks[i];
        if (!wanted.contains(t.number))
            continue;
        RipTrackJob j;
        j.number = t.number;
        const QString title = tree_->topLevelItem(i)->text(ColTitle);
        const QString albumArtist = artist_->text().trimmed();
        const QString artist = t.performer.isEmpty() ? albumArtist : t.performer;
        j.names.artist = artist;
        j.names.albumArtist = albumArtist;
        j.names.album = job.album;
        j.names.title = title;
        j.names.year = year_->text().trimmed();
        j.names.track = t.number;
        j.names.tracks = info_.tracks.size();
        j.tags.title = title;
        j.tags.artist = artist;
        j.tags.album = job.album;
        j.tags.albumArtist = albumArtist;
        j.tags.date = year_->text().trimmed();
        j.tags.isrc = t.isrc;
        j.tags.track = t.number;
        j.tags.trackTotal = info_.tracks.size();
        j.tags.cover = cover_;
        job.tracks.push_back(j);
    }
    return job;
}

void CdPage::startRip()
{
    if (busy() || info_.tracks.isEmpty())
        return;
    const RipJob job = buildJob();
    if (job.tracks.isEmpty()) {
        setStatus(QStringLiteral("Tick the tracks to copy first."));
        return;
    }
    if (job.folder.isEmpty() || !QDir(job.folder).exists()) {
        setStatus(QStringLiteral("Choose a folder that exists to copy to."));
        return;
    }
    if (info_.drive)
        stopPlaying();
    saveOptions();
    results_.clear();
    ripTotal_ = job.tracks.size();
    ripDone_ = 0;
    overall_->setValue(0);
    overall_->show();
    for (int i = 0; i < tree_->topLevelItemCount(); i++) {
        QTreeWidgetItem *it = tree_->topLevelItem(i);
        it->setData(ColCopy, kPercentRole, QVariant());
        it->setText(ColCopy, checkedTracks().contains(it->data(ColNumber, Qt::UserRole).toInt()) ? QStringLiteral("Waiting") : QString());
        it->setToolTip(ColCopy, {});
    }
    setStatus(QStringLiteral("Copying %1 tracks…").arg(ripTotal_));
    ripper_->start(job);
    setBusyControls();
}

void CdPage::stopRip()
{
    if (ripper_->running())
        ripper_->stop();
    if (imageProc_)
        imageProc_->closeWriteChannel();
    setStatus(busy() ? QStringLiteral("Stopping…") : statusText());
}

QTreeWidgetItem *CdPage::itemFor(int number) const
{
    for (int i = 0; i < tree_->topLevelItemCount(); i++)
        if (tree_->topLevelItem(i)->data(ColNumber, Qt::UserRole).toInt() == number)
            return tree_->topLevelItem(i);
    return nullptr;
}

void CdPage::markTrack(int number, int percent, const QString &text)
{
    if (QTreeWidgetItem *it = itemFor(number)) {
        it->setData(ColCopy, kPercentRole, percent);
        it->setText(ColCopy, text);
    }
}

void CdPage::chooseFolder()
{
    const QString d = QFileDialog::getExistingDirectory(this, QStringLiteral("Copy to which folder?"), folder_->text());
    if (!d.isEmpty())
        folder_->setText(d);
}

void CdPage::chooseImage()
{
    QString name = album_->text().trimmed();
    if (name.isEmpty())
        name = QStringLiteral("CD");
    const QString suggested = QDir(folder_->text()).filePath(ripRelativePath(QStringLiteral("{album}"), {QString(), QString(), name, QString(), QString(), 1, 1, 0}, QStringLiteral("cue")));
    const QString p = QFileDialog::getSaveFileName(this, QStringLiteral("Save the CD image as"), suggested, QStringLiteral("CD image (*.cue)"));
    if (!p.isEmpty())
        makeImage(p);
}

void CdPage::makeImage(const QString &cuePath)
{
    if (busy() || info_.tracks.isEmpty())
        return;
    if (info_.drive)
        stopPlaying();
    imageProc_ = new QProcess(this);
    imageProc_->setStandardErrorFile(QProcess::nullDevice());
    overall_->setValue(0);
    overall_->show();
    setStatus(QStringLiteral("Making an image of the disc…"));
    connect(imageProc_, &QProcess::readyReadStandardOutput, this, [this] {
        while (imageProc_ && imageProc_->canReadLine()) {
            const QJsonObject o = QJsonDocument::fromJson(imageProc_->readLine()).object();
            const QString ev = o.value(QStringLiteral("event")).toString();
            if (ev == QLatin1String("progress"))
                overall_->setValue(o.value(QStringLiteral("percent")).toInt());
            else if (ev == QLatin1String("done"))
                setStatus(QStringLiteral("Image made: %1. %2").arg(QDir::toNativeSeparators(o.value(QStringLiteral("path")).toString()),
                                                                   o.value(QStringLiteral("verified")).toBool() ? QStringLiteral("It opens and matches the disc.") : QStringLiteral("It didn't match the disc when opened, so check it before relying on it.")));
            else if (ev == QLatin1String("error"))
                setStatus(o.value(QStringLiteral("message")).toString());
        }
    });
    connect(imageProc_, &QProcess::finished, this, [this] {
        imageProc_->deleteLater();
        imageProc_ = nullptr;
        overall_->hide();
        setBusyControls();
        emit ripFinished();
    });
    imageProc_->start(EngineClient::enginePath(),
                      {QStringLiteral("--watch-stdin"), QStringLiteral("--cd-image"), info_.path, cuePath,
                       secure_->isChecked() ? QStringLiteral("secure") : QStringLiteral("burst"), QString::number(info_.drive ? offset_->value() : 0)},
                      QIODevice::ReadWrite);
    setBusyControls();
}

QString CdPage::updaterPath() const
{
    QString name = QStringLiteral("wittymediaplayer-updater");
#ifdef Q_OS_WIN
    name += QStringLiteral(".exe");
#endif
    return QDir(QCoreApplication::applicationDirPath()).filePath(name);
}

void CdPage::lookUp()
{
    if (lookup_ || info_.discId.isEmpty())
        return;
    setStatus(QStringLiteral("Asking MusicBrainz about this disc…"));
    lookup_ = new QProcess(this);
    lookup_->setStandardErrorFile(QProcess::nullDevice());
    connect(lookup_, &QProcess::finished, this, [this] {
        const QJsonObject o = QJsonDocument::fromJson(lookup_->readAllStandardOutput()).object();
        lookup_->deleteLater();
        lookup_ = nullptr;
        setBusyControls();
        const QJsonArray matches = o.value(QStringLiteral("matches")).toArray();
        if (o.contains(QStringLiteral("error")) || !o.contains(QStringLiteral("matches"))) {
            setStatus(QStringLiteral("MusicBrainz couldn't be reached. You can still type the names in."));
            return;
        }
        if (matches.isEmpty()) {
            setStatus(QStringLiteral("MusicBrainz doesn't know this disc yet. You can type the names in."));
            return;
        }
        int pick = 0;
        if (matches.size() > 1) {
            QStringList options;
            for (const auto &v : matches) {
                const QJsonObject m = v.toObject();
                options << QStringLiteral("%1, %2 (%3%4)").arg(m.value(QStringLiteral("artist")).toString(), m.value(QStringLiteral("title")).toString(), m.value(QStringLiteral("date")).toString().left(4),
                                                                m.value(QStringLiteral("country")).toString().isEmpty() ? QString() : QStringLiteral(", %1").arg(m.value(QStringLiteral("country")).toString()));
            }
            bool ok = false;
            const QString chosen = QInputDialog::getItem(this, QStringLiteral("Which release is this?"), QStringLiteral("MusicBrainz knows this disc by several releases:"), options, 0, false, &ok);
            if (!ok) {
                setStatus(QString());
                return;
            }
            pick = options.indexOf(chosen);
        }
        applyMatch(matches.at(pick).toObject());
    });
    lookup_->start(updaterPath(), {QStringLiteral("--cd-lookup"), info_.discId, info_.toc, QString::number(info_.tracks.size())}, QIODevice::ReadOnly);
    setBusyControls();
}

void CdPage::applyMatch(const QJsonObject &m)
{
    album_->setText(m.value(QStringLiteral("title")).toString());
    artist_->setText(m.value(QStringLiteral("artist")).toString());
    year_->setText(m.value(QStringLiteral("date")).toString().left(4));
    const QJsonArray names = m.value(QStringLiteral("tracks")).toArray();
    for (const auto &v : names) {
        const QJsonObject t = v.toObject();
        const int position = t.value(QStringLiteral("number")).toInt() - 1;
        if (position < 0 || position >= info_.tracks.size())
            continue;
        tree_->topLevelItem(position)->setText(ColTitle, t.value(QStringLiteral("title")).toString());
        info_.tracks[position].performer = t.value(QStringLiteral("artist")).toString();
    }
    showTrack();
    setStatus(QStringLiteral("Names from MusicBrainz. You can change any of them: select a track and press F2."));
    if (m.value(QStringLiteral("hasCover")).toBool())
        fetchCover(m.value(QStringLiteral("releaseId")).toString());
}

void CdPage::fetchCover(const QString &releaseId)
{
    auto *p = new QProcess(this);
    p->setStandardErrorFile(QProcess::nullDevice());
    connect(p, &QProcess::finished, this, [this, p] {
        const QJsonObject o = QJsonDocument::fromJson(p->readAllStandardOutput()).object();
        p->deleteLater();
        setCover(QByteArray::fromBase64(o.value(QStringLiteral("cover")).toString().toLatin1()));
    });
    p->start(updaterPath(), {QStringLiteral("--cd-cover"), releaseId}, QIODevice::ReadOnly);
}

void CdPage::setCover(const QByteArray &bytes)
{
    const QImage img = QImage::fromData(bytes);
    if (img.isNull())
        return;
    cover_ = bytes;
    coverView_->setImage(img);
}

} // namespace wmp
