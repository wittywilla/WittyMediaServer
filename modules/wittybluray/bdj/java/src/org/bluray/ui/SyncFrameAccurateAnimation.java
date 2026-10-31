package org.bluray.ui;

import java.awt.AlphaComposite;
import java.awt.Dimension;
import java.awt.Graphics;
import java.awt.Graphics2D;
import java.awt.image.BufferedImage;
import witty.bdj.Plane;

/**
 * Drawn frame by frame by the application, into a few buffers. A disc drawing ahead of the clock waits for its frame's
 * turn, but never for long: this is called on threads that may be doing other things.
 */
public class SyncFrameAccurateAnimation extends FrameAccurateAnimation {
    private static final int MOST_BUFFERS = 3;
    private static final long LONGEST_WAIT = 2000;
    private final BufferedImage[] buffers;
    private final long[] holds;
    private final Object lock = new Object();
    protected SyncFrameAccurateAnimation(Dimension size, int frames, AnimationParameters p) {
        super(size, p);
        int n = Math.max(1, Math.min(MOST_BUFFERS, frames));
        buffers = new BufferedImage[n];
        holds = new long[n];
        java.util.Arrays.fill(holds, -1);
    }
    public static SyncFrameAccurateAnimation getInstance(Dimension size, int frames, AnimationParameters p) { return new SyncFrameAccurateAnimation(size, frames, p); }
    public Graphics2D startDrawing(int n) { return startDrawing((long) n); }
    public void finishDrawing(int n) { finishDrawing((long) n); }
    public Graphics2D startDrawing(long n) {
        if (n < 0) return null;
        waitForRoom(n);
        int i = (int) (n % buffers.length);
        Graphics2D g;
        synchronized (lock) {
            holds[i] = -1;
            if (buffers[i] == null) buffers[i] = Plane.makeImage(Math.min(witty.bdj.Host.width, getWidth()), Math.min(witty.bdj.Host.height, getHeight()));
            g = buffers[i].createGraphics();
        }
        g.setComposite(AlphaComposite.Clear);
        g.fillRect(0, 0, buffers[i].getWidth(), buffers[i].getHeight());
        g.setComposite(AlphaComposite.SrcOver);
        return new org.dvb.ui.DVBGraphicsImpl(g);
    }
    public void finishDrawing(long n) {
        if (n < 0) return;
        synchronized (lock) { holds[(int) (n % buffers.length)] = n; }
        Plane.invalidate();
    }
    private void waitForRoom(long n) {
        long until = java.lang.System.currentTimeMillis() + LONGEST_WAIT;
        synchronized (lock) {
            while (isAnimated() && n - getCompletedFrameCount() >= buffers.length && java.lang.System.currentTimeMillis() < until) {
                try {
                    lock.wait(20);
                } catch (InterruptedException e) {
                    Thread.currentThread().interrupt();
                    return;
                }
            }
        }
    }
    public void destroy() {
        super.destroy();
        synchronized (lock) {
            java.util.Arrays.fill(buffers, null);
            java.util.Arrays.fill(holds, -1);
        }
    }
    /** The latest finished frame that is due, or failing that the earliest finished one. */
    public void paint(Graphics g) {
        BufferedImage pick = null;
        synchronized (lock) {
            long due = getCompletedFrameCount(), best = -1, first = Long.MAX_VALUE;
            BufferedImage early = null;
            for (int i = 0; i < buffers.length; i++) {
                if (holds[i] < 0) continue;
                if (holds[i] <= due && holds[i] > best) { best = holds[i]; pick = buffers[i]; }
                if (holds[i] < first) { first = holds[i]; early = buffers[i]; }
            }
            if (pick == null) pick = early;
        }
        if (pick != null) g.drawImage(pick, 0, 0, this);
    }
}
