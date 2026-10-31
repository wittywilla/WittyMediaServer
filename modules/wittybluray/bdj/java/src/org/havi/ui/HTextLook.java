package org.havi.ui;
public class HTextLook extends HDefaultLook {
    public void renderVisible(java.awt.Graphics g, HVisible v, int state) { drawText(g, v, textFor(v, state)); }
    public void renderBorders(java.awt.Graphics g, HVisible v, int state) { focusRing(g, v, state); }
    static void focusRing(java.awt.Graphics g, HVisible v, int state) {
        if (!v.getBordersEnabled() || !(v instanceof HNavigable) || (state & HState.FOCUSED_STATE_BIT) == 0) return;
        java.awt.Color c = v.getForeground();
        g.setColor(c != null ? c : java.awt.Color.white);
        g.drawRect(0, 0, v.getWidth() - 1, v.getHeight() - 1);
        g.drawRect(1, 1, v.getWidth() - 3, v.getHeight() - 3);
    }
}
