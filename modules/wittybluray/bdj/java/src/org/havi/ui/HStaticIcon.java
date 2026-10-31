package org.havi.ui;
public class HStaticIcon extends HVisible implements HNoInputPreferred {
    private static HGraphicLook look = new HGraphicLook();
    public static void setDefaultLook(HGraphicLook l) { look = l; }
    public static HGraphicLook getDefaultLook() { return look; }
    public HStaticIcon() { super(look); }
    public HStaticIcon(java.awt.Image i) { this(); setGraphicContent(i, ALL_STATES); }
    public HStaticIcon(java.awt.Image i, int x, int y, int w, int h) { this(i); setBounds(x, y, w, h); }
}
