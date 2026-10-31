package org.havi.ui;

import java.awt.Image;

public class HAnimation extends HStaticAnimation implements HNavigable {
    public HAnimation() {}
    public HAnimation(Image[] a, int delay, int mode, int repeat) { super(a, delay, mode, repeat); }
    public HAnimation(Image[] a, int delay, int mode, int repeat, int x, int y, int w, int h) { super(a, delay, mode, repeat, x, y, w, h); }
    public HAnimation(Image[] n, Image[] f, int delay, int mode, int repeat) { super(n, delay, mode, repeat); setAnimateContent(f, FOCUSED_STATE); }
    public HAnimation(Image[] n, Image[] f, int delay, int mode, int repeat, int x, int y, int w, int h) { super(n, delay, mode, repeat, x, y, w, h); setAnimateContent(f, FOCUSED_STATE); }
}
