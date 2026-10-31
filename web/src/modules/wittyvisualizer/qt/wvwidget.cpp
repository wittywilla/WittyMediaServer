#include "wvwidget.h"

#include <QCheckBox>
#include <QComboBox>
#include <QDialog>
#include <QDialogButtonBox>
#include <QFormLayout>
#include <QHBoxLayout>
#include <QKeyEvent>
#include <QLabel>
#include <QPainter>
#include <QPainterPath>
#include <QPushButton>
#include <QRandomGenerator>
#include <QSlider>
#include <QStyle>
#include <QTimer>
#include <QToolButton>
#include <QVBoxLayout>
#include <cmath>

namespace wv {

namespace {

// Icons drawn from the same paths the web version uses.
QIcon pathIcon(const QColor &color, const std::function<void(QPainterPath &)> &make)
{
    QPixmap pm(48, 48);
    pm.fill(Qt::transparent);
    QPainter p(&pm);
    p.setRenderHint(QPainter::Antialiasing);
    p.scale(2, 2);
    QPainterPath path;
    make(path);
    p.fillPath(path, color);
    p.end();
    return QIcon(pm);
}

void rectPath(QPainterPath &p, qreal x, qreal y, qreal w, qreal h) { p.addRect(x, y, w, h); }

QToolButton *iconButton(QWidget *parent, const QString &label, const QIcon &icon)
{
    auto *b = new QToolButton(parent);
    b->setIcon(icon);
    b->setIconSize(QSize(22, 22));
    b->setMinimumSize(36, 36);
    b->setAutoRaise(true);
    b->setToolTip(label);
    b->setAccessibleName(label);
    return b;
}

QString scaleLabel(double v) { return v == 0 ? QStringLiteral("Automatic (recommended)") : QStringLiteral("%1%").arg(qRound(v * 100)); }
QString everyLabel(int s)
{
    if (s == 0)
        return QStringLiteral("Stay on one effect");
    if (s < 60)
        return QStringLiteral("Every %1 seconds").arg(s);
    if (s == 60)
        return QStringLiteral("Every minute");
    return QStringLiteral("Every %1 minutes").arg(s / 60);
}

} // namespace

// --- the picture

VisualizerView::VisualizerView(SampleTap *tap, Settings *settings, QWidget *parent)
    : QRhiWidget(parent), tap_(tap), settings_(settings), palettes_(builtinPalettes())
{
    setAttribute(Qt::WA_OpaquePaintEvent);
    setFocusPolicy(Qt::StrongFocus);
    setAccessibleName(QStringLiteral("Visualizer picture"));
    setAccessibleDescription(QStringLiteral("Moving light that follows the music. It has nothing to read."));
    left_.resize(kFftSize);
    right_.resize(kFftSize);
    analyzer_.sensitivity = settings->sensitivity;
}

bool VisualizerView::calm() const { return settings_->calm >= 0 ? settings_->calm == 1 : systemWantsLessMotion(); }

QStringList VisualizerView::inRunning() const
{
    return eligible(builtinEffects(), *settings_, calm(), software_ || (budget_.frameGap > 0 && budget_.scale <= budget_.floor() + 1e-3));
}

void VisualizerView::resetBudget()
{
    const double chosen = settings_->scale;
    const double start = chosen > 0 ? chosen : software_ ? 0.33 : 1;
    budget_ = Budget(start, chosen > 0 ? chosen : software_ ? 0.2 : 0.25, chosen > 0 ? chosen : 1);
    if (software_)
        budget_.frameGap = 1.0 / 30;
}

void VisualizerView::settingsChanged()
{
    analyzer_.sensitivity = settings_->sensitivity;
    analyzer_.minBeatGap = calm() ? 0.5 : kMinBeatGap;
    resetBudget();
}

void VisualizerView::setRunning(bool on)
{
    running_ = on;
    tap_->setEnabled(on);
    last_ = std::chrono::steady_clock::now();
    if (on)
        update();
}

void VisualizerView::initialize(QRhiCommandBuffer *)
{
    problem_.clear();
    ready_ = renderer_.init(rhi(), renderTarget()->renderPassDescriptor());
    if (!ready_) {
        problem_ = renderer_.error();
        return;
    }
    const QString device = QString::fromLatin1(rhi()->driverInfo().deviceName);
    software_ = device.contains(QStringLiteral("Basic Render"), Qt::CaseInsensitive) || device.contains(QStringLiteral("llvmpipe"), Qt::CaseInsensitive)
        || device.contains(QStringLiteral("SwiftShader"), Qt::CaseInsensitive) || device.contains(QStringLiteral("WARP"), Qt::CaseInsensitive);
    resetBudget();
    if (fromId_.isEmpty()) {
        const QStringList ids = inRunning();
        const QString saved = settings_->effect;
        if (settings_->cycleSec == 0 && ids.contains(saved)) {
            cycle_.select(saved);
            fromId_ = saved;
        } else {
            fromId_ = cycle_.next(ids, settings_->random);
        }
        emit effectChanged(fromId_);
    }
    last_ = std::chrono::steady_clock::now();
}

void VisualizerView::releaseResources()
{
    renderer_.release();
    ready_ = false;
}

void VisualizerView::next() { change(cycle_.next(inRunning(), settings_->random)); }
void VisualizerView::previous() { change(cycle_.previous(inRunning(), settings_->random)); }
void VisualizerView::select(const QString &id)
{
    cycle_.select(id);
    change(id);
}

void VisualizerView::change(const QString &id)
{
    if (id.isEmpty())
        return;
    if (!toId_.isEmpty())
        finishMix();
    if (id == fromId_)
        return;
    toId_ = id;
    mix_ = 0;
    mixStyle_ = QRandomGenerator::global()->bounded(4);
    mixFor_ = calm() ? 4.0 : 2.4;
    shownFor_ = 0;
    emit effectChanged(id);
}

void VisualizerView::finishMix()
{
    if (toId_.isEmpty())
        return;
    renderer_.releaseTarget(fromId_);
    fromId_ = toId_;
    toId_.clear();
    mix_ = 0;
}

// When this path isn't carrying the sound (the VLC backend plays its own), the picture
// follows slow swells and a soft pulse instead, the same pattern as in tap.ts.
void VisualizerView::readAudio(double dt)
{
    const int rate = tap_->rate();
    const bool moving = tap_->read(left_.data(), right_.data());
    if (moving)
        everHeard_ = true;
    quietFrames_ = moving ? 0 : quietFrames_ + 1;
    hearing_ = everHeard_ || quietFrames_ < 60;
    if (hearing_) {
        analyzer_.process(left_.data(), right_.data(), rate, dt);
        return;
    }
    const double slow = patternAt_;
    const double low = 0.25 + 0.2 * std::sin(slow * 0.7), middle = 0.12 + 0.1 * std::sin(slow * 1.1 + 2), high = 0.05 + 0.04 * std::sin(slow * 1.7 + 4);
    const double two = 2 * 3.14159265358979;
    for (int i = 0; i < kFftSize; i++) {
        const double t = patternAt_ + double(i - kFftSize) / 44100;
        const double phase = std::fmod(t * 1.2, 1.0);
        const double pulse = std::exp(-(phase < 0 ? phase + 1 : phase) * 7);
        left_[size_t(i)] = float(std::sin(t * two * 70) * low * pulse + std::sin(t * two * 440) * middle + std::sin(t * two * 3500) * high);
        right_[size_t(i)] = float(std::sin(t * two * 70) * low * pulse + std::sin(t * two * 660) * middle + std::sin(t * two * 2500) * high);
    }
    patternAt_ += dt;
    analyzer_.process(left_.data(), right_.data(), 44100, dt);
}

void VisualizerView::render(QRhiCommandBuffer *cb)
{
    const QColor black(Qt::black);
    if (!ready_ || !running_) {
        QRhiResourceUpdateBatch *u = rhi()->nextResourceUpdateBatch();
        cb->beginPass(renderTarget(), black, {1.0f, 0}, u);
        cb->endPass();
        return;
    }
    // The next frame. This stops by itself when the widget isn't on screen.
    if (budget_.frameGap > 0)
        QTimer::singleShot(int(budget_.frameGap * 1000), this, [this] { if (running_) update(); });
    else
        update();

    const auto now = std::chrono::steady_clock::now();
    const double dt = std::max(0.001, std::min(0.25, std::chrono::duration<double>(now - last_).count()));
    last_ = now;

    readAudio(dt);
    const AudioFrame &a = analyzer_.frame();
    const bool isCalm = calm();
    const double intensity = settings_->intensity;
    time_ += dt * (isCalm ? 0.6 : 1);
    flow_ += dt * (0.3 + 0.75 * a.level * intensity + 0.5 * a.beat) * (isCalm ? 0.45 : 1);
    palettes_.step(dt, a.beatCount, isCalm);

    shownFor_ += dt;
    if (settings_->cycleSec > 0 && toId_.isEmpty() && shownFor_ >= settings_->cycleSec)
        next();

    const auto infoOf = [](const QString &id) {
        for (const EffectInfo &e : builtinEffects())
            if (e.id == id)
                return e;
        return EffectInfo{};
    };
    if (!renderer_.prepare(infoOf(fromId_))) {
        problem_ = renderer_.error();
    }
    QString incoming;
    if (!toId_.isEmpty()) {
        if (renderer_.prepare(infoOf(toId_))) {
            incoming = toId_;
            mix_ = std::min(1.0, mix_ + dt / mixFor_);
        } else {
            toId_.clear();
        }
    }

    // Effects draw at the budget's share of the widget's pixels, never past a long edge of
    // 1920, and the present pass stretches them.
    const QSize full = renderTarget()->pixelSize();
    const double fit = std::min(1.0, 1920.0 / std::max(full.width(), full.height()));
    renderer_.setSize(QSize(qRound(full.width() * fit * budget_.scale), qRound(full.height() * fit * budget_.scale)));

    DrawInput f;
    f.audio = &a;
    f.time = time_;
    f.flow = flow_;
    f.dt = dt;
    f.intensity = intensity;
    f.calm = isCalm;
    f.palette = palettes_.out;
    f.guard = isCalm ? kGuardCalm : kGuard;
    f.a = fromId_;
    f.b = incoming;
    f.mix = mix_ * mix_ * (3 - 2 * mix_);
    f.mixStyle = mixStyle_;
    renderer_.draw(cb, renderTarget(), f, black);
    if (!toId_.isEmpty() && mix_ >= 1)
        finishMix();

    budget_.frame(dt);
    const double cost = std::chrono::duration<double>(std::chrono::steady_clock::now() - now).count();
    costSum_ += cost;
    statsFrames_++;
    statsFor_ += dt;
    if (statsFor_ >= 1) {
        fps_ = statsFrames_ / statsFor_;
        frameMs_ = costSum_ * 1000 / statsFrames_;
        statsFrames_ = 0;
        statsFor_ = 0;
        costSum_ = 0;
        emit statsChanged();
    }
}

// --- the whole thing

VisualizerWidget::VisualizerWidget(SampleTap *tap, QSettings *store, QWidget *parent)
    : QWidget(parent), tap_(tap), store_(store)
{
    setAccessibleName(QStringLiteral("Visualizer"));
    settings_.load(*store_);
    view_ = new VisualizerView(tap_, &settings_, this);
    stats_ = new QLabel(view_);
    stats_->setVisible(settings_.stats);
    stats_->move(10, 10);
    stats_->setStyleSheet(QStringLiteral("background: rgba(0,0,0,0.75); color: white; padding: 3px 6px;"));

    bar_ = new QWidget(this);
    bar_->setAccessibleName(QStringLiteral("Visualizer controls"));
    auto *row = new QHBoxLayout(bar_);
    row->setContentsMargins(6, 3, 6, 3);
    row->setSpacing(4);
    const QColor ink = palette().color(QPalette::ButtonText);
    auto *prev = iconButton(bar_, QStringLiteral("Previous effect"), pathIcon(ink, [](QPainterPath &p) { rectPath(p, 6, 6, 2, 12); p.moveTo(9.5, 12); p.lineTo(18, 18); p.lineTo(18, 6); p.closeSubpath(); }));
    auto *next = iconButton(bar_, QStringLiteral("Next effect"), pathIcon(ink, [](QPainterPath &p) { rectPath(p, 16, 6, 2, 12); p.moveTo(6, 18); p.lineTo(14.5, 12); p.lineTo(6, 6); p.closeSubpath(); }));
    effects_ = new QComboBox(bar_);
    effects_->setAccessibleName(QStringLiteral("Effect"));
    for (const EffectInfo &e : builtinEffects())
        effects_->addItem(e.name, e.id);
    star_ = iconButton(bar_, QStringLiteral("Favourite"), pathIcon(ink, [](QPainterPath &p) { p.moveTo(12, 2.6); p.lineTo(14.9, 8.9); p.lineTo(21.8, 9.7); p.lineTo(16.7, 14.4); p.lineTo(18.1, 21.2); p.lineTo(12, 17.8); p.lineTo(5.9, 21.2); p.lineTo(7.3, 14.4); p.lineTo(2.2, 9.7); p.lineTo(9.1, 8.9); p.closeSubpath(); }));
    star_->setCheckable(true);
    auto *gear = iconButton(bar_, QStringLiteral("Visualizer settings"), style()->standardIcon(QStyle::SP_FileDialogDetailedView));
    auto *full = iconButton(bar_, QStringLiteral("Full screen"), pathIcon(ink, [](QPainterPath &p) { rectPath(p, 5, 5, 5, 2); rectPath(p, 5, 5, 2, 5); rectPath(p, 14, 5, 5, 2); rectPath(p, 17, 5, 2, 5); rectPath(p, 5, 17, 5, 2); rectPath(p, 5, 14, 2, 5); rectPath(p, 14, 17, 5, 2); rectPath(p, 17, 14, 2, 5); }));
    auto *off = iconButton(bar_, QStringLiteral("Turn the visualizer off"), style()->standardIcon(QStyle::SP_TitleBarCloseButton));
    row->addWidget(prev);
    row->addWidget(effects_);
    row->addWidget(next);
    row->addWidget(star_);
    row->addStretch(1);
    row->addWidget(gear);
    row->addWidget(full);
    row->addWidget(off);

    auto *col = new QVBoxLayout(this);
    col->setContentsMargins(0, 0, 0, 0);
    col->setSpacing(0);
    col->addWidget(view_, 1);
    col->addWidget(bar_);

    connect(prev, &QToolButton::clicked, view_, &VisualizerView::previous);
    connect(next, &QToolButton::clicked, view_, &VisualizerView::next);
    connect(effects_, &QComboBox::activated, this, [this](int i) { view_->select(effects_->itemData(i).toString()); });
    connect(star_, &QToolButton::clicked, this, [this] { handleKey(Qt::Key_S); });
    connect(gear, &QToolButton::clicked, this, &VisualizerWidget::openSettings);
    connect(full, &QToolButton::clicked, this, &VisualizerWidget::fullscreenRequested);
    connect(off, &QToolButton::clicked, this, &VisualizerWidget::closeRequested);
    connect(view_, &VisualizerView::effectChanged, this, &VisualizerWidget::refresh);
    connect(view_, &VisualizerView::statsChanged, this, [this] {
        QStringList lines;
        if (!view_->problem().isEmpty())
            lines << view_->problem();
        if (!view_->hearing())
            lines << QStringLiteral("The sound can't be read on this playback path, so the picture is following a pattern.");
        if (settings_.stats)
            lines << QStringLiteral("%1 fps · %2×%3 · %4% · %5 ms").arg(qRound(view_->fps())).arg(view_->drawSize().width()).arg(view_->drawSize().height()).arg(qRound(view_->scale() * 100)).arg(view_->frameMs(), 0, 'f', 1);
        stats_->setVisible(!lines.isEmpty());
        stats_->setText(lines.join(QLatin1Char('\n')));
        stats_->adjustSize();
    });
}

VisualizerWidget::~VisualizerWidget()
{
    tap_->setEnabled(false);
    settings_.save(*store_);
}

QString VisualizerWidget::effectId() const { return view_->current(); }
void VisualizerWidget::setControlsVisible(bool on) { bar_->setVisible(on); }
void VisualizerWidget::saveSoon() { settings_.save(*store_); }

void VisualizerWidget::showEvent(QShowEvent *e)
{
    QWidget::showEvent(e);
    view_->setRunning(true);
}

void VisualizerWidget::hideEvent(QHideEvent *e)
{
    QWidget::hideEvent(e);
    // Nothing runs while it's off: no frames, and the audio thread stops copying.
    view_->setRunning(false);
    saveSoon();
}

void VisualizerWidget::refresh()
{
    const QString id = view_->current();
    const int at = effects_->findData(id);
    if (at >= 0)
        effects_->setCurrentIndex(at);
    star_->setChecked(settings_.favourites.contains(id));
    settings_.effect = id;
}

bool VisualizerWidget::handleKey(int key)
{
    switch (key) {
    case Qt::Key_Left:
    case Qt::Key_P:
        view_->previous();
        return true;
    case Qt::Key_Right:
    case Qt::Key_N:
        view_->next();
        return true;
    case Qt::Key_R:
        settings_.random = !settings_.random;
        saveSoon();
        return true;
    case Qt::Key_S: {
        const QString id = view_->current();
        if (settings_.favourites.contains(id))
            settings_.favourites.removeAll(id);
        else
            settings_.favourites << id;
        star_->setChecked(settings_.favourites.contains(id));
        saveSoon();
        return true;
    }
    case Qt::Key_Plus:
    case Qt::Key_Equal:
        settings_.intensity = std::min(1.5, settings_.intensity + 0.1);
        saveSoon();
        return true;
    case Qt::Key_Minus:
        settings_.intensity = std::max(0.3, settings_.intensity - 0.1);
        saveSoon();
        return true;
    default:
        return false;
    }
}

bool VisualizerWidget::confirmFirstUse()
{
    if (settings_.notice)
        return true;
    QDialog d(window());
    d.setWindowTitle(QStringLiteral("Before you turn this on"));
    auto *col = new QVBoxLayout(&d);
    auto *text = new QLabel(QStringLiteral(
        "<p>The visualizer fills the screen with moving, pulsing light that follows the music.</p>"
        "<p><b>Flashing and fast-moving patterns can trigger seizures in people with photosensitive epilepsy</b>, and can make other people feel unwell. It limits flashes by design, but that can't make it safe for everyone.</p>"
        "<p>If you, or anyone who might be watching, is sensitive to this, leave it off. If you feel unwell, look away and turn it off.</p>"), &d);
    text->setWordWrap(true);
    text->setMinimumWidth(440);
    col->addWidget(text);
    auto *calm = new QCheckBox(QStringLiteral("Use calm mode: slower, softer, and only the gentle effects"), &d);
    calm->setChecked(settings_.calm >= 0 ? settings_.calm == 1 : systemWantsLessMotion());
    col->addWidget(calm);
    auto *buttons = new QDialogButtonBox(&d);
    auto *no = buttons->addButton(QStringLiteral("Not now"), QDialogButtonBox::RejectRole);
    buttons->addButton(QStringLiteral("Turn on"), QDialogButtonBox::AcceptRole);
    no->setDefault(true);
    connect(buttons, &QDialogButtonBox::accepted, &d, &QDialog::accept);
    connect(buttons, &QDialogButtonBox::rejected, &d, &QDialog::reject);
    col->addWidget(buttons);
    if (d.exec() != QDialog::Accepted)
        return false;
    settings_.notice = true;
    settings_.on = true;
    settings_.calm = calm->isChecked() ? 1 : (systemWantsLessMotion() ? 0 : -1);
    view_->settingsChanged();
    saveSoon();
    return true;
}

void VisualizerWidget::openSettings()
{
    QDialog d(window());
    d.setWindowTitle(QStringLiteral("Visualizer"));
    auto *form = new QFormLayout(&d);
    auto *cycle = new QComboBox(&d);
    cycle->setAccessibleName(QStringLiteral("Change effect"));
    for (int c : cycleChoices())
        cycle->addItem(everyLabel(c), c);
    cycle->setCurrentIndex(cycle->findData(settings_.cycleSec));
    form->addRow(QStringLiteral("Change effect"), cycle);
    auto *random = new QCheckBox(QStringLiteral("Random order"), &d);
    random->setChecked(settings_.random);
    form->addRow(random);
    auto *favs = new QCheckBox(QStringLiteral("Only my favourites (mark some with the star)"), &d);
    favs->setChecked(settings_.favouritesOnly);
    form->addRow(favs);
    const auto slider = [&](const QString &name, double value, int lo, int hi) {
        auto *s = new QSlider(Qt::Horizontal, &d);
        s->setRange(lo, hi);
        s->setSingleStep(5);
        s->setValue(qRound(value * 100));
        s->setAccessibleName(name);
        form->addRow(name, s);
        return s;
    };
    auto *strength = slider(QStringLiteral("How strongly it reacts"), settings_.intensity, 30, 150);
    auto *hearing = slider(QStringLiteral("How easily it hears the music"), settings_.sensitivity, 25, 200);
    auto *calm = new QComboBox(&d);
    calm->setAccessibleName(QStringLiteral("Calm mode"));
    calm->addItem(QStringLiteral("Follow this computer's animation setting"), -1);
    calm->addItem(QStringLiteral("On"), 1);
    calm->addItem(QStringLiteral("Off"), 0);
    calm->setCurrentIndex(calm->findData(settings_.calm));
    calm->setToolTip(QStringLiteral("Slower, softer, and only the gentle effects."));
    form->addRow(QStringLiteral("Calm mode"), calm);
    auto *res = new QComboBox(&d);
    res->setAccessibleName(QStringLiteral("Resolution"));
    for (double v : scaleChoices())
        res->addItem(scaleLabel(v), v);
    res->setCurrentIndex(res->findData(settings_.scale));
    form->addRow(QStringLiteral("Resolution"), res);
    auto *on = new QCheckBox(QStringLiteral("Show it when I play music"), &d);
    on->setChecked(settings_.on);
    form->addRow(on);
    auto *stats = new QCheckBox(QStringLiteral("Show frame rate and resolution"), &d);
    stats->setChecked(settings_.stats);
    form->addRow(stats);
    auto *keys = new QLabel(QStringLiteral("Keys: Left and Right or P and N change effect, R random, S favourite, F full screen, + and − strength, V off."), &d);
    keys->setWordWrap(true);
    form->addRow(keys);
    auto *close = new QDialogButtonBox(QDialogButtonBox::Close, &d);
    connect(close, &QDialogButtonBox::rejected, &d, &QDialog::accept);
    form->addRow(close);

    const auto apply = [&, this] {
        settings_.cycleSec = cycle->currentData().toInt();
        settings_.random = random->isChecked();
        settings_.favouritesOnly = favs->isChecked();
        settings_.intensity = strength->value() / 100.0;
        settings_.sensitivity = hearing->value() / 100.0;
        settings_.calm = calm->currentData().toInt();
        settings_.scale = res->currentData().toDouble();
        settings_.on = on->isChecked();
        settings_.stats = stats->isChecked();
        stats_->setVisible(settings_.stats);
        view_->settingsChanged();
        saveSoon();
    };
    for (auto *c : {cycle, calm, res})
        connect(c, &QComboBox::currentIndexChanged, &d, apply);
    for (auto *c : {random, favs, on, stats})
        connect(c, &QCheckBox::toggled, &d, apply);
    for (auto *s : {strength, hearing})
        connect(s, &QSlider::valueChanged, &d, apply);
    d.exec();
}

} // namespace wv
