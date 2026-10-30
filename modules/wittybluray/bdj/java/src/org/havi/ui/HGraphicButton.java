package org.havi.ui;
public class HGraphicButton extends HButtonBase {
    public HGraphicButton() { super(new HGraphicLook()); }
    public HGraphicButton(java.awt.Image i) { this(); setGraphicContent(i, ALL_STATES); }
    public HGraphicButton(java.awt.Image i, int x, int y, int w, int h) { this(i); setBounds(x, y, w, h); }
    public HGraphicButton(java.awt.Image n, java.awt.Image f, java.awt.Image a) { this(n); setGraphicContent(f, FOCUSED_STATE); setGraphicContent(a, ACTIONED_STATE); }
    public HGraphicButton(java.awt.Image n, java.awt.Image f, java.awt.Image a, int x, int y, int w, int h) { this(n, f, a); setBounds(x, y, w, h); }
}
