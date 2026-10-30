package org.havi.ui;
public class HIcon extends HButtonBase {
    public HIcon() { super(new HGraphicLook()); }
    public HIcon(java.awt.Image i) { this(); setGraphicContent(i, ALL_STATES); }
    public HIcon(java.awt.Image i, int x, int y, int w, int h) { this(i); setBounds(x, y, w, h); }
}
