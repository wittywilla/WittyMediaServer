package org.havi.ui;

/** The running of an HAnimateEffect, on one shared daemon timer that never runs disc code. */
abstract class HEffect {
    private static final java.util.Timer timer = new java.util.Timer("havi-effects", true);
    private int position, delay = 1, mode = HAnimateEffect.PLAY_REPEATING, repeat = HAnimateEffect.REPEAT_INFINITE, way = 1, done;
    private java.util.TimerTask task;
    abstract int frames();
    abstract void shown();
    final synchronized void start() {
        if (task != null || frames() <= 0) return;
        done = 0;
        way = 1;
        schedule();
    }
    private void schedule() {
        task = new java.util.TimerTask() {
            public void run() {
                try {
                    step();
                } catch (Throwable t) {
                    cancel();
                }
            }
        };
        long gap = Math.max(40L, delay * 100L);
        try {
            timer.schedule(task, gap, gap);
        } catch (IllegalStateException e) {
            task = null; // disc code stopped the timer's thread: the picture just stands still
        }
    }
    final synchronized void stop() {
        if (task != null) task.cancel();
        task = null;
    }
    final synchronized boolean running() { return task != null; }
    private synchronized void step() {
        int n = frames();
        if (n <= 1) return;
        int p = position + way;
        if (mode == HAnimateEffect.PLAY_ALTERNATING) {
            if (p < 0 || p >= n) {
                way = -way;
                p = position + way;
                if (way > 0 && ended()) return;
            }
        } else if (p >= n) {
            if (ended()) return;
            p = 0;
        }
        position = Math.max(0, Math.min(n - 1, p));
        shown();
    }
    private boolean ended() {
        if (repeat == HAnimateEffect.REPEAT_INFINITE || ++done < repeat) return false;
        stop();
        return true;
    }
    final synchronized void position(int p) {
        int n = frames();
        position = n <= 0 ? 0 : Math.max(0, Math.min(n - 1, p));
        shown();
    }
    final synchronized int position() { return position; }
    final synchronized void delay(int d) {
        delay = Math.max(1, d);
        if (task != null) {
            task.cancel();
            schedule();
        }
    }
    final synchronized int delay() { return delay; }
    final synchronized void mode(int m) { if (m == HAnimateEffect.PLAY_REPEATING || m == HAnimateEffect.PLAY_ALTERNATING) mode = m; }
    final synchronized int mode() { return mode; }
    final synchronized void repeat(int r) { repeat = r <= 0 ? HAnimateEffect.REPEAT_INFINITE : r; done = 0; }
    final synchronized int repeat() { return repeat; }
    /** Runs something on the event queue a little later. */
    static void after(long ms, final Runnable r) {
        try {
            timer.schedule(new java.util.TimerTask() { public void run() { HComponent.later(r); } }, ms);
        } catch (IllegalStateException e) {
            HComponent.later(r);
        }
    }
}
