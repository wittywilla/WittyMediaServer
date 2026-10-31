package org.havi.ui;
public class HIcon extends HStaticIcon implements HNavigable {
    public HIcon() {}
    public HIcon(java.awt.Image i) { super(i); }
    public HIcon(java.awt.Image i, int x, int y, int w, int h) { super(i, x, y, w, h); }
    public HIcon(java.awt.Image n, java.awt.Image f) { super(n); setGraphicContent(f, FOCUSED_STATE); }
    public HIcon(java.awt.Image n, java.awt.Image f, int x, int y, int w, int h) { super(n, x, y, w, h); setGraphicContent(f, FOCUSED_STATE); }
}
