package org.havi.ui;
public class HStaticAnimation extends HVisible {
    public static final int PLAY_ALTERNATING = 1, PLAY_REPEATING = 2, REPEAT_INFINITE = -1;
    private int pos, delay = 1, mode = PLAY_REPEATING, repeat = REPEAT_INFINITE;
    private volatile boolean run;
    public HStaticAnimation() { super(new HAnimateLook()); }
    public HStaticAnimation(java.awt.Image[] a, int delay, int mode, int repeat) { this(); setAnimateContent(a, ALL_STATES); this.delay = delay; this.mode = mode; this.repeat = repeat; }
    public HStaticAnimation(java.awt.Image[] a, int delay, int mode, int repeat, int x, int y, int w, int h) { this(a, delay, mode, repeat); setBounds(x, y, w, h); }
    public void start() {
        final java.awt.Image[] a = getAnimateContent(NORMAL_STATE);
        if (a == null || a.length == 0 || run) return;
        run = true;
        Thread t = new Thread(new Runnable() { public void run() {
            while (HStaticAnimation.this.run) {
                pos = (pos + 1) % a.length;
                setGraphicContent(a[pos], ALL_STATES);
                try { Thread.sleep(Math.max(40, delay * 100L)); } catch (InterruptedException e) { return; }
            }
        } });
        t.setDaemon(true);
        t.start();
    }
    public void stop() { run = false; }
    public boolean isAnimated() { return run; }
    public void setPosition(int p) { pos = p; }
    public int getPosition() { return pos; }
    public void setRepeatCount(int c) { repeat = c; }
    public int getRepeatCount() { return repeat; }
    public void setDelay(int d) { delay = d; }
    public int getDelay() { return delay; }
    public void setPlayMode(int m) { mode = m; }
    public int getPlayMode() { return mode; }
}
