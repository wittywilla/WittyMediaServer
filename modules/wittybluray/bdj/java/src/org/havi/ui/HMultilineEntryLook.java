package org.havi.ui;

import java.awt.FontMetrics;
import java.awt.Graphics;

/** Lines of the entry's text; it breaks only where the text has a line break. */
public class HMultilineEntryLook extends HSinglelineEntryLook {
    public void renderVisible(Graphics g, HVisible v, int state) {
        if (!(v instanceof HMultilineEntry)) { super.renderVisible(g, v, state); return; }
        HMultilineEntry e = (HMultilineEntry) v;
        begin(g, v);
        FontMetrics m = g.getFontMetrics();
        String t = shown(e);
        int line = 0, start = 0, caret = e.getCaretCharPosition(), h = m.getHeight();
        for (int i = 0; i <= t.length(); i++) {
            if (i < t.length() && t.charAt(i) != '\n') continue;
            int y = 2 + line * h + m.getAscent();
            g.drawString(t.substring(start, i), 2, y);
            if (e.getEditMode() && caret >= start && caret <= i) {
                int x = 2 + m.stringWidth(t.substring(start, caret));
                g.drawLine(x, y - m.getAscent(), x, y + m.getDescent());
            }
            line++;
            start = i + 1;
        }
    }
    public int[] getSoftLineBreaks(HVisible v) { return new int[0]; }
    public int getCaretPositionNextLine(HVisible v) { return v instanceof HMultilineEntry ? ((HMultilineEntry) v).caretOnLine(1) : 0; }
    public int getCaretPositionPreviousLine(HVisible v) { return v instanceof HMultilineEntry ? ((HMultilineEntry) v).caretOnLine(-1) : 0; }
}
