package org.bluray.ui;
public class AnimationParameters {
    public int threadPriority = 5, scaleFactor = 1;
    public int[] repeatCount;
    public boolean lockedToVideo;
    public FrameAccurateAnimationTimer faaTimer;
    public AnimationParameters() {}
    public AnimationParameters(int priority, int scale, int[] repeat, boolean locked, FrameAccurateAnimationTimer timer) { threadPriority = priority; scaleFactor = scale; repeatCount = repeat; lockedToVideo = locked; faaTimer = timer; }
}
