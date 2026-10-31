// The look of the window: the project's dark themes, as Qt style sheets and drawn
// icons. The colours come from the same theme files WittyMediaServer uses.
#pragma once

#include <QColor>
#include <QHash>
#include <QIcon>
#include <QString>
#include <QStringList>

class QApplication;

namespace wmp {

struct Theme {
    QString id, name;
    QColor bg, bg2, bg3, bg4;
    QColor fg, fg2, fg3;
    QColor accent, accentHover, accentFg, accentText;
    QColor danger, ok, warn, controlLine, focus;
    int radius = 10;
};

// How two colours read against each other: 1 to 21.
double contrastRatio(const QColor &a, const QColor &b);

class Themes {
public:
    static QStringList ids();
    static Theme load(const QString &id);
    static QString styleSheet(const Theme &t);
    static void apply(QApplication *app, const Theme &t);
    static const Theme &current();
    // An icon from the small drawn set: play pause prev next volume mute fullscreen
    // exitfullscreen audio subtitles menu title stop open disc.
    static QIcon icon(const QString &name);
};

} // namespace wmp
