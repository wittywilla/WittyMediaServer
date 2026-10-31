package org.bluray.ui;

import witty.bdj.Plane;
import witty.bdj.Playback;

/**
 * An animation synchronised to the video. Which frame is due comes from the clock, video's or wall, at the video's
 * rate; the plane shows the latest of them at its own, lower rate, so a small machine drops frames rather than falls behind.
 */
public abstract class FrameAccurateAnimation extends org.havi.ui.HComponent {
    static final double RATE = 24000.0 / 1001;
    private static final java.util.Timer timer = new java.util.Timer("bdj-animations", true);
    protected final AnimationParameters params;
    private final Object state = new Object();
    private java.util.TimerTask task;
    private volatile boolean running;
    private volatile boolean attached;
    private long startedAt;
    private double startedVideo;
    private long lastFrame = -1;
    protected FrameAccurateAnimation(java.awt.Dimension size, AnimationParameters p) {
        params = p != null ? p : new AnimationParameters();
        if (size != null) setSize(size);
    }
    public AnimationParameters getAnimationParameters() { return params; }
    public int getThreadPriority() { return params.threadPriority; }
    public void setThreadPriority(int p) { params.threadPriority = p; }
    public void addNotify() { super.addNotify(); attached = true; }
    public void removeNotify() { attached = false; stop(); super.removeNotify(); }
    public void start() {
        synchronized (state) {
            if (running) return;
            running = true;
            lastFrame = -1;
            startedAt = java.lang.System.nanoTime();
            startedVideo = Playback.now();
            task = new java.util.TimerTask() {
                public void run() {
                    try {
                        if (!running) { cancel(); return; }
                        long f = due();
                        if (f != lastFrame) {
                            lastFrame = f;
                            if (attached) Plane.invalidate();
                        }
                    } catch (Throwable t) {
                        cancel();
                    }
                }
            };
            try {
                long gap = 1000 / Plane.FPS;
                timer.schedule(task, 0, gap);
            } catch (IllegalStateException e) {
                task = null; // disc code stopped the timer's thread
            }
        }
    }
    public void stop() {
        synchronized (state) {
            running = false;
            if (task != null) task.cancel();
            task = null;
        }
    }
    public boolean isAnimated() { return running; }
    public void destroy() { stop(); }
    /** How many frames are due since the animation started. */
    public long getCompletedFrameCount() { return due(); }
    private long due() {
        double t;
        FrameAccurateAnimationTimer w = params.faaTimer;
        if (w != null) {
            double now = Playback.now(), end = w.stopSeconds();
            t = Math.max(0, (end >= 0 ? Math.min(now, end) : now) - w.startSeconds());
        } else if (params.lockedToVideo) {
            t = Math.max(0, Playback.now() - startedVideo);
        } else {
            t = running ? (java.lang.System.nanoTime() - startedAt) / 1e9 : 0;
        }
        return (long) (t * RATE / Math.max(1, params.scaleFactor));
    }
}
