package org.havi.ui;
public class HDefaultTextLayoutManager implements HTextLayoutManager {
    public void render(String t, java.awt.Graphics g, HVisible v, java.awt.Insets i) {
        java.awt.FontMetrics m = g.getFontMetrics();
        g.drawString(t, i == null ? 0 : i.left, (v.getHeight() + m.getAscent() - m.getDescent()) / 2);
    }
    public java.awt.Dimension getMinimumSize(HVisible v) { return new java.awt.Dimension(0, 0); }
    public java.awt.Dimension getMaximumSize(HVisible v) { return new java.awt.Dimension(32767, 32767); }
    public java.awt.Dimension getPreferredSize(HVisible v) { return v.getSize(); }
}
