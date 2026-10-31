package org.havi.ui;

import java.awt.Image;

public class HStaticAnimation extends HVisible implements HNoInputPreferred, HAnimateEffect {
    private static HAnimateLook look = new HAnimateLook();
    public static void setDefaultLook(HAnimateLook l) { look = l; }
    public static HAnimateLook getDefaultLook() { return look; }
    private volatile boolean attached;
    private final HEffect effect = new HEffect() {
        int frames() { return frameCount(); }
        void shown() { if (attached) witty.bdj.Plane.invalidate(); }
    };
    public HStaticAnimation() { super(look); }
    public HStaticAnimation(Image[] a, int delay, int mode, int repeat) { this(); setAnimateContent(a, ALL_STATES); setDelay(delay); setPlayMode(mode); setRepeatCount(repeat); }
    public HStaticAnimation(Image[] a, int delay, int mode, int repeat, int x, int y, int w, int h) { this(a, delay, mode, repeat); setBounds(x, y, w, h); }
    public void addNotify() { super.addNotify(); attached = true; }
    public void removeNotify() { attached = false; effect.stop(); super.removeNotify(); }
    public void start() { effect.start(); }
    public void stop() { effect.stop(); }
    public boolean isAnimated() { return effect.running(); }
    public void setPosition(int p) { effect.position(p); }
    public int getPosition() { return effect.position(); }
    public void setRepeatCount(int c) { effect.repeat(c); }
    public int getRepeatCount() { return effect.repeat(); }
    public void setDelay(int d) { effect.delay(d); }
    public int getDelay() { return effect.delay(); }
    public void setPlayMode(int m) { effect.mode(m); }
    public int getPlayMode() { return effect.mode(); }
}
