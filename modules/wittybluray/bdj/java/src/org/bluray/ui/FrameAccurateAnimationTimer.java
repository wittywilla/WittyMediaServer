package org.bluray.ui;
/** The stretch of the video's time an animation runs in; a negative end means until it is stopped. */
public class FrameAccurateAnimationTimer {
    private final double start, stop;
    protected FrameAccurateAnimationTimer(double from, double to) { start = from; stop = to; }
    public static FrameAccurateAnimationTimer getInstance(javax.media.Time start, javax.media.Time stop) {
        return new FrameAccurateAnimationTimer(start == null ? 0 : start.getSeconds(), stop == null ? -1 : stop.getSeconds());
    }
    public static FrameAccurateAnimationTimer getInstance(int startFrame, int stopFrame) {
        return new FrameAccurateAnimationTimer(startFrame / FrameAccurateAnimation.RATE, stopFrame < 0 ? -1 : stopFrame / FrameAccurateAnimation.RATE);
    }
    public javax.media.Time getAnimationStartTime() { return new javax.media.Time(start); }
    public javax.media.Time getAnimationStopTime() { return new javax.media.Time(Math.max(0, stop)); }
    final double startSeconds() { return start; }
    final double stopSeconds() { return stop; }
    public void start() {}
    public void stop() {}
}
