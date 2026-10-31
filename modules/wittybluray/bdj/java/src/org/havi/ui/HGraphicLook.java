package org.havi.ui;
public class HGraphicLook extends HDefaultLook {
    public void renderVisible(java.awt.Graphics g, HVisible v, int state) { drawGraphic(g, v, graphicFor(v, state)); }
}
