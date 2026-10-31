#include "subtitlepaint.h"

#include <QFont>
#include <QFontMetrics>
#include <QPainter>
#include <QPainterPath>
#include <QTextLayout>

namespace wmp {

Overlay textOverlay(const QString &text, const QSize &frame, uint64_t id)
{
    // Drawn on a canvas the shape of the picture, so it scales with the window.
    const int w = 1280;
    const int h = frame.width() > 0 && frame.height() > 0 ? w * frame.height() / frame.width() : 720;
    const int pixel = qMax(24, h / 18);
    QFont font(QStringLiteral("Segoe UI"));
    font.setPixelSize(pixel);
    font.setWeight(QFont::Medium);
    const QFontMetrics fm(font);
    const int maxWidth = w * 9 / 10;

    // Lay out the lines, wrapping long ones.
    struct Line {
        QString text;
    };
    QVector<Line> lines;
    const QStringList paras = text.split(QLatin1Char('\n'));
    for (const QString &para : paras) {
        QString rest = para.trimmed();
        while (!rest.isEmpty()) {
            if (fm.horizontalAdvance(rest) <= maxWidth) {
                lines.push_back({rest});
                break;
            }
            int cut = rest.size();
            while (cut > 0 && (fm.horizontalAdvance(rest.left(cut)) > maxWidth || rest[cut - 1] != QLatin1Char(' ')))
                cut--;
            if (cut <= 0) {
                cut = rest.size();
                while (cut > 1 && fm.horizontalAdvance(rest.left(cut)) > maxWidth)
                    cut--;
            }
            lines.push_back({rest.left(cut).trimmed()});
            rest = rest.mid(cut).trimmed();
        }
    }
    const int lineH = fm.lineSpacing();
    const int boxH = lineH * int(lines.size()) + pixel / 2;
    QImage img(w, boxH, QImage::Format_RGBA8888_Premultiplied);
    img.fill(Qt::transparent);
    {
        QPainter p(&img);
        p.setRenderHint(QPainter::Antialiasing);
        QPainterPath path;
        int y = fm.ascent() + pixel / 4;
        for (const Line &l : lines) {
            const int x = (w - fm.horizontalAdvance(l.text)) / 2;
            path.addText(x, y, font, l.text);
            y += lineH;
        }
        p.setPen(QPen(QColor(0, 0, 0, 220), pixel / 7.0, Qt::SolidLine, Qt::RoundCap, Qt::RoundJoin));
        p.drawPath(path);
        p.fillPath(path, Qt::white);
    }
    const double boxFrac = double(boxH) / h;
    Overlay o;
    o.image = img;
    o.rect = QRectF(0, 0.94 - boxFrac, 1, boxFrac);
    o.key = (quint64(1) << 63) | id;
    return o;
}

} // namespace wmp
