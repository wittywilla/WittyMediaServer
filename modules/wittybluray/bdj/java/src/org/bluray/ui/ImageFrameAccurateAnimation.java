package org.bluray.ui;

import java.awt.Dimension;
import java.awt.Graphics;
import java.awt.Image;

/** A list of pictures, each shown for its count of frames. */
public class ImageFrameAccurateAnimation extends FrameAccurateAnimation {
    public static final int PLAY_REPEATING = 1, PLAY_ALTERNATING = 2, PLAY_ONCE = 3;
    private final Image[] images;
    private volatile int mode;
    protected ImageFrameAccurateAnimation(Image[] i, Dimension size, AnimationParameters p, int m) { super(size, p); images = i == null ? new Image[0] : (Image[]) i.clone(); mode = m; }
    public static ImageFrameAccurateAnimation getInstance(Image[] i, Dimension size, AnimationParameters p, int mode) { return new ImageFrameAccurateAnimation(i, size, p, mode); }
    public Image[] getImages() { return (Image[]) images.clone(); }
    public int getPlayMode() { return mode; }
    public void setPlayMode(int m) { if (m >= PLAY_REPEATING && m <= PLAY_ONCE) mode = m; }
    public void prepareImages() {}
    public boolean isImagesPrepared() { return true; }
    private int hold(int i) { int[] r = params.repeatCount; return r == null || i >= r.length ? 1 : Math.max(1, r[i]); }
    /** The picture a frame number falls on. */
    private int pictureAt(long frame) {
        long total = 0;
        for (int i = 0; i < images.length; i++) total += hold(i);
        if (total <= 0) return -1;
        long at;
        if (mode == PLAY_ONCE) {
            at = Math.min(frame, total - 1);
        } else if (mode == PLAY_ALTERNATING && total > 1) {
            long p = frame % (2 * total - 2);
            at = p < total ? p : 2 * total - 2 - p;
        } else {
            at = frame % total;
        }
        for (int i = 0; i < images.length; i++) {
            at -= hold(i);
            if (at < 0) return i;
        }
        return images.length - 1;
    }
    public void paint(Graphics g) {
        int i = pictureAt(getCompletedFrameCount());
        if (i >= 0 && images[i] != null) g.drawImage(images[i], 0, 0, this);
    }
}
