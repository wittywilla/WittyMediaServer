#include "theme.h"

#include <QApplication>
#include <QFile>
#include <QJsonDocument>
#include <QJsonObject>
#include <QPainter>
#include <QPainterPath>
#include <QPalette>
#include <QPixmap>
#include <cmath>

namespace wmp {

namespace {
Theme g_current;

// Theme files write translucent colours as #rrggbbaa; only opaque ones are used here.
QColor hexColor(const QJsonObject &c, const char *key, const QColor &fallback)
{
    const QString v = c.value(QLatin1String(key)).toString();
    if (v.size() != 7 && v.size() != 9)
        return fallback;
    QColor out(v.left(7));
    return out.isValid() ? out : fallback;
}

double channel(double v)
{
    v /= 255.0;
    return v <= 0.03928 ? v / 12.92 : std::pow((v + 0.055) / 1.055, 2.4);
}

double luminance(const QColor &c)
{
    return 0.2126 * channel(c.red()) + 0.7152 * channel(c.green()) + 0.0722 * channel(c.blue());
}
} // namespace

double contrastRatio(const QColor &a, const QColor &b)
{
    double x = luminance(a), y = luminance(b);
    if (x < y)
        std::swap(x, y);
    return (x + 0.05) / (y + 0.05);
}

QStringList Themes::ids()
{
    return {QStringLiteral("dark"), QStringLiteral("oled"), QStringLiteral("dark-blue"), QStringLiteral("dark-teal"), QStringLiteral("dark-green"),
            QStringLiteral("dark-gold"), QStringLiteral("dark-orange"), QStringLiteral("dark-red"), QStringLiteral("dark-pink"), QStringLiteral("dark-purple")};
}

Theme Themes::load(const QString &id)
{
    Theme t;
    t.id = id;
    QFile f(QStringLiteral(":/themes/%1.json").arg(id));
    QJsonObject o;
    if (f.open(QIODevice::ReadOnly))
        o = QJsonDocument::fromJson(f.readAll()).object();
    const QJsonObject c = o.value(QStringLiteral("colors")).toObject();
    t.name = o.value(QStringLiteral("name")).toString(id);
    t.bg = hexColor(c, "bg", QColor(0x101114));
    t.bg2 = hexColor(c, "bg2", QColor(0x17191e));
    t.bg3 = hexColor(c, "bg3", QColor(0x20232a));
    t.bg4 = hexColor(c, "bg4", QColor(0x2b2f38));
    t.fg = hexColor(c, "fg", QColor(0xececf1));
    t.fg2 = hexColor(c, "fg2", QColor(0xa8adb9));
    t.fg3 = hexColor(c, "fg3", QColor(0x858a99));
    t.accent = hexColor(c, "accent", QColor(0xe4e6eb));
    t.accentHover = hexColor(c, "accentHover", t.accent);
    t.accentFg = hexColor(c, "accentFg", QColor(0x111214));
    t.accentText = hexColor(c, "accentText", t.accent);
    t.danger = hexColor(c, "danger", QColor(0xff5c6c));
    t.ok = hexColor(c, "ok", QColor(0x3ad29f));
    t.warn = hexColor(c, "warn", QColor(0xe0a030));
    t.controlLine = hexColor(c, "controlLine", QColor(0x6f7480));
    t.focus = hexColor(c, "focus", t.accent);
    t.radius = o.value(QStringLiteral("radius")).toInt(10);
    return t;
}

QString Themes::styleSheet(const Theme &t)
{
    const auto h = [](const QColor &c) { return c.name(QColor::HexRgb); };
    QString s = QStringLiteral(R"(
* { font-family: "Segoe UI Variable Text", "Segoe UI", system-ui, sans-serif; font-size: 14px; }
QMainWindow, QDialog, QWidget#central, QWidget#startPage { background: @bg; color: @fg; }
QWidget#controlBar { background: @bg2; }
QLabel { color: @fg; background: transparent; }
QLabel[role="muted"] { color: @fg2; }
QLabel[role="hint"] { color: @fg3; font-size: 13px; }
QLabel[role="brand"] { font-size: 38px; font-weight: 650; }
QLabel[role="heading"] { font-size: 17px; font-weight: 600; }
QLabel[role="time"] { color: @fg2; font-size: 13px; }
QLabel[role="message"] { font-size: 15px; }
QWidget#messageBox { background: @bg2; border: 1px solid @bg4; border-radius: @radiuspx; }
QWidget#driveRow { background: @bg3; border-radius: @radiuspx; }
QMenuBar { background: @bg2; color: @fg2; padding: 2px 6px; }
QMenuBar::item { background: transparent; padding: 6px 10px; border-radius: 6px; }
QMenuBar::item:selected { background: @bg3; color: @fg; }
QMenu { background: @bg2; color: @fg; border: 1px solid @bg4; padding: 6px; }
QMenu::item { padding: 7px 28px 7px 24px; border-radius: 6px; }
QMenu::item:selected { background: @bg3; }
QMenu::item:disabled { color: @fg3; }
QMenu::separator { height: 1px; background: @bg4; margin: 6px 8px; }
QMenu::indicator { width: 14px; height: 14px; left: 6px; }
QRadioButton, QCheckBox { color: @fg; spacing: 8px; }
QCheckBox:focus { color: @accentText; }
QRadioButton::indicator, QCheckBox::indicator { width: 14px; height: 14px; background: @bg3; border: 2px solid @line; }
QRadioButton::indicator { border-radius: 9px; }
QCheckBox::indicator { border-radius: 4px; }
QRadioButton::indicator:checked, QCheckBox::indicator:checked { background: @accent; border-color: @accent; }
QRadioButton:focus::indicator, QCheckBox:focus::indicator { border-color: @focus; }
QFrame[frameShape="4"] { color: @bg4; }
QRadioButton:focus { color: @accentText; }
QLineEdit { background: @bg3; color: @fg; border: 1px solid @line; border-radius: 6px; padding: 6px 8px; }
QLineEdit:focus { border: 2px solid @focus; padding: 5px 7px; }
QLineEdit:disabled { color: @fg3; border-color: @bg4; }
QPushButton { background: @bg3; color: @fg; border: 1px solid @bg4; border-radius: @radiuspx; padding: 8px 16px; }
QPushButton:hover { background: @bg4; }
QPushButton:pressed { background: @bg4; border-color: @line; }
QPushButton:disabled { color: @fg3; }
QPushButton:focus { border: 2px solid @focus; padding: 7px 15px; }
QPushButton[role="primary"] { background: @accent; color: @accentFg; border-color: @accent; font-weight: 600; }
QPushButton[role="primary"]:hover { background: @accentHover; }
QToolButton { background: transparent; color: @fg; border: none; border-radius: 8px; padding: 6px; }
QToolButton:hover { background: @bg3; }
QToolButton:pressed { background: @bg4; }
QToolButton:disabled { color: @fg3; }
QToolButton:focus { border: 2px solid @focus; padding: 4px; }
QToolButton::menu-indicator { image: none; }
QProgressBar { background: @bg3; border: 1px solid @bg4; border-radius: 6px; text-align: center; color: @fg; min-height: 20px; }
QProgressBar::chunk { background: @accent; border-radius: 5px; }
QToolTip { background: @bg3; color: @fg; border: 1px solid @bg4; padding: 4px 8px; }
QScrollBar:vertical { background: @bg; width: 10px; }
QScrollBar::handle:vertical { background: @bg4; border-radius: 5px; min-height: 24px; }
QScrollBar::add-line:vertical, QScrollBar::sub-line:vertical { height: 0; }
)");
    const QVector<QPair<QString, QColor>> colors = {
        {QStringLiteral("@bg4"), t.bg4}, {QStringLiteral("@bg3"), t.bg3}, {QStringLiteral("@bg2"), t.bg2}, {QStringLiteral("@bg"), t.bg},
        {QStringLiteral("@fg3"), t.fg3}, {QStringLiteral("@fg2"), t.fg2}, {QStringLiteral("@fg"), t.fg},
        {QStringLiteral("@accentHover"), t.accentHover}, {QStringLiteral("@accentFg"), t.accentFg}, {QStringLiteral("@accentText"), t.accentText}, {QStringLiteral("@accent"), t.accent},
        {QStringLiteral("@line"), t.controlLine}, {QStringLiteral("@focus"), t.focus},
    };
    for (const auto &c : colors)
        s.replace(c.first, h(c.second));
    s.replace(QStringLiteral("@radius"), QString::number(t.radius));
    return s;
}

const Theme &Themes::current()
{
    if (g_current.id.isEmpty())
        g_current = load(QStringLiteral("dark"));
    return g_current;
}

void Themes::apply(QApplication *app, const Theme &t)
{
    g_current = t;
    QPalette p;
    p.setColor(QPalette::Window, t.bg);
    p.setColor(QPalette::WindowText, t.fg);
    p.setColor(QPalette::Base, t.bg3);
    p.setColor(QPalette::AlternateBase, t.bg2);
    p.setColor(QPalette::Text, t.fg);
    p.setColor(QPalette::Button, t.bg3);
    p.setColor(QPalette::ButtonText, t.fg);
    p.setColor(QPalette::Highlight, t.accent);
    p.setColor(QPalette::HighlightedText, t.accentFg);
    p.setColor(QPalette::ToolTipBase, t.bg3);
    p.setColor(QPalette::ToolTipText, t.fg);
    p.setColor(QPalette::PlaceholderText, t.fg3);
    p.setColor(QPalette::Disabled, QPalette::Text, t.fg3);
    p.setColor(QPalette::Disabled, QPalette::ButtonText, t.fg3);
    p.setColor(QPalette::Disabled, QPalette::WindowText, t.fg3);
    app->setPalette(p);
    app->setStyleSheet(styleSheet(t));
}

QIcon Themes::icon(const QString &name)
{
    const int size = 48;
    QPixmap pm(size, size);
    pm.fill(Qt::transparent);
    QPainter p(&pm);
    p.setRenderHint(QPainter::Antialiasing);
    const QColor c = current().fg;
    p.setBrush(c);
    p.setPen(QPen(c, 4, Qt::SolidLine, Qt::RoundCap, Qt::RoundJoin));
    const auto poly = [&](std::initializer_list<QPointF> pts) { p.drawPolygon(QPolygonF(QList<QPointF>(pts))); };
    const auto line = [&](qreal a, qreal b, qreal x, qreal y) { p.drawLine(QPointF(a, b), QPointF(x, y)); };
    if (name == QLatin1String("play")) {
        poly({{15, 9}, {38, 24}, {15, 39}});
    } else if (name == QLatin1String("pause")) {
        p.drawRoundedRect(QRectF(12, 9, 8, 30), 2, 2);
        p.drawRoundedRect(QRectF(28, 9, 8, 30), 2, 2);
    } else if (name == QLatin1String("stop")) {
        p.drawRoundedRect(QRectF(12, 12, 24, 24), 3, 3);
    } else if (name == QLatin1String("prev")) {
        poly({{36, 10}, {18, 24}, {36, 38}});
        line(12, 10, 12, 38);
    } else if (name == QLatin1String("next")) {
        poly({{12, 10}, {30, 24}, {12, 38}});
        line(36, 10, 36, 38);
    } else if (name == QLatin1String("volume") || name == QLatin1String("mute")) {
        poly({{7, 19}, {15, 19}, {25, 10}, {25, 38}, {15, 29}, {7, 29}});
        p.setBrush(Qt::NoBrush);
        if (name == QLatin1String("volume")) {
            p.drawArc(QRectF(20, 15, 14, 18), -60 * 16, 120 * 16);
            p.drawArc(QRectF(20, 8, 22, 32), -60 * 16, 120 * 16);
        } else {
            line(32, 18, 42, 30);
            line(42, 18, 32, 30);
        }
    } else if (name == QLatin1String("fullscreen") || name == QLatin1String("exitfullscreen")) {
        p.setBrush(Qt::NoBrush);
        const bool in = name == QLatin1String("exitfullscreen");
        const auto corner = [&](qreal x, qreal y, qreal dx, qreal dy) {
            const qreal s = in ? -1 : 1;
            const qreal ax = in ? x + dx * 10 : x, ay = in ? y + dy * 10 : y;
            line(ax, ay, ax + s * dx * 10, ay);
            line(ax, ay, ax, ay + s * dy * 10);
        };
        corner(9, 9, 1, 1);
        corner(39, 9, -1, 1);
        corner(9, 39, 1, -1);
        corner(39, 39, -1, -1);
    } else if (name == QLatin1String("audio")) {
        p.setBrush(Qt::NoBrush);
        line(18, 34, 18, 10);
        line(18, 10, 38, 7);
        line(38, 7, 38, 31);
        p.setBrush(c);
        p.drawEllipse(QPointF(13, 35), 5, 4.5);
        p.drawEllipse(QPointF(33, 32), 5, 4.5);
    } else if (name == QLatin1String("subtitles")) {
        p.setBrush(Qt::NoBrush);
        p.drawRoundedRect(QRectF(6, 11, 36, 26), 4, 4);
        line(13, 25, 22, 25);
        line(26, 25, 35, 25);
        line(13, 31, 28, 31);
    } else if (name == QLatin1String("menu")) {
        line(9, 13, 39, 13);
        line(9, 24, 39, 24);
        line(9, 35, 39, 35);
    } else if (name == QLatin1String("title")) {
        p.setBrush(Qt::NoBrush);
        p.drawRoundedRect(QRectF(7, 9, 34, 30), 4, 4);
        line(7, 18, 41, 18);
        line(16, 9, 16, 18);
        line(27, 9, 27, 18);
    } else if (name == QLatin1String("open")) {
        p.setBrush(Qt::NoBrush);
        p.drawPolygon(QPolygonF(QList<QPointF>{{6, 12}, {19, 12}, {23, 17}, {42, 17}, {42, 38}, {6, 38}}));
    } else if (name == QLatin1String("disc")) {
        p.setBrush(Qt::NoBrush);
        p.drawEllipse(QPointF(24, 24), 18, 18);
        p.setBrush(c);
        p.drawEllipse(QPointF(24, 24), 4, 4);
    }
    p.end();
    return QIcon(pm);
}

} // namespace wmp
