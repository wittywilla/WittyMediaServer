package org.havi.ui;
public class HGraphicButton extends HIcon implements HActionable {
    public HGraphicButton() {}
    public HGraphicButton(java.awt.Image i) { super(i); }
    public HGraphicButton(java.awt.Image i, int x, int y, int w, int h) { super(i, x, y, w, h); }
    public HGraphicButton(java.awt.Image n, java.awt.Image f, java.awt.Image a) { super(n, f); actioned(a); }
    public HGraphicButton(java.awt.Image n, java.awt.Image f, java.awt.Image a, int x, int y, int w, int h) { super(n, f, x, y, w, h); actioned(a); }
    private void actioned(java.awt.Image a) { setGraphicContent(a, ACTIONED_STATE); setGraphicContent(a, ACTIONED_FOCUSED_STATE); }
}
