#include "controlbar.h"

#include "theme.h"

#include <QAccessible>
#include <QHBoxLayout>
#include <QKeyEvent>
#include <QLabel>
#include <QMouseEvent>
#include <QPainter>
#include <QStyleOptionSlider>
#include <QVBoxLayout>

namespace wmp {

SeekBar::SeekBar(QWidget *parent) : QSlider(Qt::Horizontal, parent)
{
    setObjectName(QStringLiteral("seek"));
    setRange(0, 0);
    setSingleStep(10000);
    setPageStep(60000);
    setMinimumHeight(22);
    setFocusPolicy(Qt::StrongFocus);
    setAccessibleName(QStringLiteral("Position"));
    setMouseTracking(false);
}

void SeekBar::setDuration(double seconds)
{
    duration_ = seconds;
    setRange(0, qMax(0, int(seconds * 1000)));
}

void SeekBar::setPosition(double seconds)
{
    if (down_)
        return;
    const QSignalBlocker block(this);
    setValue(int(seconds * 1000));
    update();
}

void SeekBar::setChapters(const QVector<double> &chapters)
{
    chapters_ = chapters;
    update();
}

double SeekBar::secondsAt(int x) const
{
    if (width() <= 12 || duration_ <= 0)
        return 0;
    return qBound(0.0, double(x - 6) / (width() - 12), 1.0) * duration_;
}

void SeekBar::mousePressEvent(QMouseEvent *e)
{
    if (e->button() != Qt::LeftButton || duration_ <= 0)
        return;
    down_ = true;
    setSliderDown(true);
    setValue(int(secondsAt(int(e->position().x())) * 1000));
    update();
}

void SeekBar::mouseMoveEvent(QMouseEvent *e)
{
    if (down_) {
        setValue(int(secondsAt(int(e->position().x())) * 1000));
        update();
    }
}

void SeekBar::mouseReleaseEvent(QMouseEvent *e)
{
    if (!down_)
        return;
    down_ = false;
    setSliderDown(false);
    emit seekRequested(secondsAt(int(e->position().x())));
    update();
}

void SeekBar::keyPressEvent(QKeyEvent *e)
{
    const double now = valueSeconds();
    switch (e->key()) {
    case Qt::Key_Left:
        emit seekRequested(qMax(0.0, now - 10));
        return;
    case Qt::Key_Right:
        emit seekRequested(now + 10);
        return;
    case Qt::Key_Home:
        emit seekRequested(0);
        return;
    case Qt::Key_End:
        emit seekRequested(qMax(0.0, duration_ - 10));
        return;
    default:
        QWidget::keyPressEvent(e);
    }
}

void SeekBar::paintEvent(QPaintEvent *)
{
    QPainter p(this);
    p.setRenderHint(QPainter::Antialiasing);
    const Theme &t = Themes::current();
    const int m = 6, y = height() / 2, w = width() - 2 * m;
    const bool on = isEnabled() && duration_ > 0;
    const int h = hasFocus() ? 6 : 4;
    p.setPen(Qt::NoPen);
    p.setBrush(t.bg4);
    p.drawRoundedRect(QRectF(m, y - h / 2.0, w, h), h / 2.0, h / 2.0);
    const double frac = on ? double(value()) / qMax(1, maximum()) : 0;
    p.setBrush(on ? t.accent : t.fg3);
    p.drawRoundedRect(QRectF(m, y - h / 2.0, w * frac, h), h / 2.0, h / 2.0);
    if (on) {
        // Chapter marks cut small gaps in the line.
        for (double c : chapters_) {
            if (c <= 0.5 || c >= duration_)
                continue;
            const double x = m + w * c / duration_;
            p.setBrush(t.bg2);
            p.drawRect(QRectF(x - 1, y - h / 2.0, 2, h));
        }
        p.setBrush(hasFocus() ? t.focus : t.fg);
        const double r = hasFocus() || down_ ? 7 : 5.5;
        p.drawEllipse(QPointF(m + w * frac, y), r, r);
    }
}

VolumeBar::VolumeBar(QWidget *parent) : QSlider(Qt::Horizontal, parent)
{
    setObjectName(QStringLiteral("volume"));
    setRange(0, 100);
    setSingleStep(5);
    setPageStep(10);
    setFixedSize(96, 22);
    setAccessibleName(QStringLiteral("Volume"));
    setFocusPolicy(Qt::TabFocus);
}

void VolumeBar::mousePressEvent(QMouseEvent *e)
{
    if (e->button() == Qt::LeftButton)
        setValue(int(100.0 * (e->position().x() - 6) / (width() - 12)));
}

void VolumeBar::mouseMoveEvent(QMouseEvent *e)
{
    if (e->buttons() & Qt::LeftButton)
        setValue(int(100.0 * (e->position().x() - 6) / (width() - 12)));
}

void VolumeBar::paintEvent(QPaintEvent *)
{
    QPainter p(this);
    p.setRenderHint(QPainter::Antialiasing);
    const Theme &t = Themes::current();
    const int m = 6, y = height() / 2, w = width() - 2 * m, h = hasFocus() ? 6 : 4;
    const double frac = double(value()) / 100.0;
    p.setPen(Qt::NoPen);
    p.setBrush(t.bg4);
    p.drawRoundedRect(QRectF(m, y - h / 2.0, w, h), h / 2.0, h / 2.0);
    p.setBrush(t.accent);
    p.drawRoundedRect(QRectF(m, y - h / 2.0, w * frac, h), h / 2.0, h / 2.0);
    p.setBrush(hasFocus() ? t.focus : t.fg);
    p.drawEllipse(QPointF(m + w * frac, y), hasFocus() ? 7 : 5.5, hasFocus() ? 7 : 5.5);
}

QString ControlBar::formatTime(double seconds)
{
    if (seconds < 0 || qIsNaN(seconds))
        seconds = 0;
    const int s = int(seconds);
    const int h = s / 3600, m = (s % 3600) / 60, r = s % 60;
    if (h > 0)
        return QStringLiteral("%1:%2:%3").arg(h).arg(m, 2, 10, QLatin1Char('0')).arg(r, 2, 10, QLatin1Char('0'));
    return QStringLiteral("%1:%2").arg(m).arg(r, 2, 10, QLatin1Char('0'));
}

QToolButton *ControlBar::tool(const QString &icon, const QString &name, const QString &tip)
{
    auto *b = new QToolButton(this);
    b->setIcon(Themes::icon(icon));
    b->setIconSize(QSize(20, 20));
    b->setAccessibleName(name);
    b->setToolTip(tip);
    b->setFocusPolicy(Qt::TabFocus);
    b->setMinimumSize(36, 36);
    return b;
}

ControlBar::ControlBar(QWidget *parent) : QWidget(parent)
{
    setObjectName(QStringLiteral("controlBar"));
    setAttribute(Qt::WA_StyledBackground);
    auto *col = new QVBoxLayout(this);
    col->setContentsMargins(12, 6, 12, 8);
    col->setSpacing(2);

    auto *top = new QHBoxLayout;
    top->setSpacing(10);
    time_ = new QLabel(QStringLiteral("0:00"), this);
    time_->setProperty("role", QStringLiteral("time"));
    total_ = new QLabel(QStringLiteral("0:00"), this);
    total_->setProperty("role", QStringLiteral("time"));
    seek_ = new SeekBar(this);
    top->addWidget(time_);
    top->addWidget(seek_, 1);
    top->addWidget(total_);
    col->addLayout(top);

    auto *row = new QHBoxLayout;
    row->setSpacing(4);
    prev_ = tool(QStringLiteral("prev"), QStringLiteral("Previous chapter"), QStringLiteral("Previous chapter (P)"));
    play_ = tool(QStringLiteral("play"), QStringLiteral("Play"), QStringLiteral("Play or pause (Space)"));
    play_->setIconSize(QSize(24, 24));
    next_ = tool(QStringLiteral("next"), QStringLiteral("Next chapter"), QStringLiteral("Next chapter (N)"));
    mute_ = tool(QStringLiteral("volume"), QStringLiteral("Mute"), QStringLiteral("Mute"));
    volume_ = new VolumeBar(this);
    menu_ = tool(QStringLiteral("menu"), QStringLiteral("Disc menu"), QStringLiteral("Disc menu (M)"));
    title_ = tool(QStringLiteral("title"), QStringLiteral("Title menu"), QStringLiteral("Title menu (T)"));
    audio_ = tool(QStringLiteral("audio"), QStringLiteral("Audio track"), QStringLiteral("Audio track (A)"));
    subs_ = tool(QStringLiteral("subtitles"), QStringLiteral("Subtitles"), QStringLiteral("Subtitles (S)"));
    audio_->setPopupMode(QToolButton::InstantPopup);
    subs_->setPopupMode(QToolButton::InstantPopup);
    full_ = tool(QStringLiteral("fullscreen"), QStringLiteral("Full screen"), QStringLiteral("Full screen (F)"));

    row->addWidget(prev_);
    row->addWidget(play_);
    row->addWidget(next_);
    row->addSpacing(8);
    row->addWidget(mute_);
    row->addWidget(volume_);
    row->addStretch(1);
    row->addWidget(menu_);
    row->addWidget(title_);
    row->addSpacing(8);
    row->addWidget(audio_);
    row->addWidget(subs_);
    row->addWidget(full_);
    col->addLayout(row);

    connect(play_, &QToolButton::clicked, this, &ControlBar::playPause);
    connect(prev_, &QToolButton::clicked, this, &ControlBar::previous);
    connect(next_, &QToolButton::clicked, this, &ControlBar::next);
    connect(mute_, &QToolButton::clicked, this, &ControlBar::muteToggled);
    connect(menu_, &QToolButton::clicked, this, &ControlBar::rootMenu);
    connect(title_, &QToolButton::clicked, this, &ControlBar::titleMenu);
    connect(full_, &QToolButton::clicked, this, &ControlBar::fullscreen);
    connect(volume_, &QSlider::valueChanged, this, &ControlBar::volumeChanged);
    connect(seek_, &SeekBar::seekRequested, this, &ControlBar::seekTo);
    connect(seek_, &QSlider::valueChanged, this, [this](int ms) {
        time_->setText(formatTime(ms / 1000.0));
    });
    setDiscMode(false);
    setHasMedia(false);
}

void ControlBar::setPlaying(bool playing)
{
    if (playing == playing_)
        return;
    playing_ = playing;
    play_->setIcon(Themes::icon(playing ? QStringLiteral("pause") : QStringLiteral("play")));
    play_->setAccessibleName(playing ? QStringLiteral("Pause") : QStringLiteral("Play"));
}

void ControlBar::setPosition(double pos, double dur)
{
    if (std::abs(seek_->duration() - dur) > 0.01)
        seek_->setDuration(dur);
    if (seek_->dragging())
        return;
    seek_->setPosition(pos);
    const QString t = formatTime(pos), d = formatTime(dur);
    if (time_->text() != t)
        time_->setText(t);
    if (total_->text() != d)
        total_->setText(d);
}

void ControlBar::setChapters(const QVector<double> &chapters)
{
    seek_->setChapters(chapters);
}

void ControlBar::setVolume(int percent, bool muted)
{
    const QSignalBlocker block(volume_);
    volume_->setValue(percent);
    mute_->setIcon(Themes::icon(muted || percent == 0 ? QStringLiteral("mute") : QStringLiteral("volume")));
    mute_->setAccessibleName(muted ? QStringLiteral("Unmute") : QStringLiteral("Mute"));
}

void ControlBar::setFullscreen(bool on)
{
    full_->setIcon(Themes::icon(on ? QStringLiteral("exitfullscreen") : QStringLiteral("fullscreen")));
    full_->setAccessibleName(on ? QStringLiteral("Leave full screen") : QStringLiteral("Full screen"));
}

void ControlBar::setDiscMode(bool disc)
{
    menu_->setVisible(disc);
    title_->setVisible(disc);
}

void ControlBar::setSeekable(bool on)
{
    seek_->setEnabled(on);
}

void ControlBar::setHasMedia(bool on)
{
    for (QWidget *w : {static_cast<QWidget *>(play_), static_cast<QWidget *>(prev_), static_cast<QWidget *>(next_), static_cast<QWidget *>(seek_)})
        w->setEnabled(on);
    if (!on) {
        seek_->setDuration(0);
        time_->setText(QStringLiteral("0:00"));
        total_->setText(QStringLiteral("0:00"));
        seek_->setChapters({});
    }
}

void ControlBar::setHasTracks(bool audio, bool subs)
{
    audio_->setEnabled(audio);
    subs_->setEnabled(subs);
}

} // namespace wmp
