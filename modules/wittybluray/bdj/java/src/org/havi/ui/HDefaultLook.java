package org.havi.ui;

import java.awt.Dimension;
import java.awt.Graphics;
import java.awt.Image;
import java.awt.Insets;

/** Draws a visible's picture and text: the looks disc authors get without writing their own. */
public class HDefaultLook implements HLook {
    public void showLook(Graphics g, HVisible v, int state) {
        if (v.getBackgroundMode() == HVisible.BACKGROUND_FILL) {
            g.setColor(v.getBackground());
            g.fillRect(0, 0, v.getWidth(), v.getHeight());
        }
        Image i = v.getGraphicContent(state);
        if (i != null) g.drawImage(i, 0, 0, v);
        String t = v.getTextContent(state);
        if (t != null) {
            g.setColor(v.getForeground() != null ? v.getForeground() : java.awt.Color.white);
            if (v.getFont() != null) g.setFont(v.getFont());
            java.awt.FontMetrics m = g.getFontMetrics();
            int y = (v.getHeight() + m.getAscent() - m.getDescent()) / 2;
            int x = (v.getWidth() - m.stringWidth(t)) / 2;
            g.drawString(t, Math.max(0, x), y);
        }
    }
    public void widgetChanged(HVisible v, HChangeData[] c) { v.repaint(); }
    public Dimension getMinimumSize(HVisible v) { return new Dimension(0, 0); }
    public Dimension getPreferredSize(HVisible v) { return v.getSize(); }
    public Dimension getMaximumSize(HVisible v) { return new Dimension(32767, 32767); }
    public boolean isOpaque(HVisible v) { return false; }
    public Insets getInsets(HVisible v) { return new Insets(0, 0, 0, 0); }
}
