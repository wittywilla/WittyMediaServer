package org.havi.ui;

import java.awt.Color;
import java.awt.FontMetrics;
import java.awt.Graphics;

/** The entry's text with a caret while it is being edited. */
public class HSinglelineEntryLook extends HDefaultLook {
    public void renderBorders(Graphics g, HVisible v, int state) { HTextLook.focusRing(g, v, state); }
    public void renderVisible(Graphics g, HVisible v, int state) {
        if (!(v instanceof HSinglelineEntry)) { super.renderVisible(g, v, state); return; }
        HSinglelineEntry e = (HSinglelineEntry) v;
        begin(g, v);
        FontMetrics m = g.getFontMetrics();
        String t = shown(e);
        int y = (v.getHeight() + m.getAscent() - m.getDescent()) / 2;
        g.drawString(t, 2, y);
        if (e.getEditMode()) {
            int p = Math.min(t.length(), e.getCaretCharPosition());
            int x = 2 + m.stringWidth(t.substring(0, p));
            g.drawLine(x, y - m.getAscent(), x, y + m.getDescent());
        }
    }
    static void begin(Graphics g, HVisible v) {
        Color c = v.getForeground();
        g.setColor(c != null ? c : Color.white);
        if (v.getFont() != null) g.setFont(v.getFont());
    }
    static String shown(HSinglelineEntry e) {
        String t = e.getText();
        if (!e.echoCharIsSet()) return t;
        char[] a = new char[t.length()];
        java.util.Arrays.fill(a, e.getEchoChar());
        return new String(a);
    }
}
