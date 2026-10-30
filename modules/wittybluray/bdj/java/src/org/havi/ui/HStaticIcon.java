package org.havi.ui;
public class HStaticIcon extends HVisible {
    public HStaticIcon() { super(new HGraphicLook()); }
    public HStaticIcon(java.awt.Image i) { this(); setGraphicContent(i, ALL_STATES); }
    public HStaticIcon(java.awt.Image i, int x, int y, int w, int h) { this(i); setBounds(x, y, w, h); }
}
