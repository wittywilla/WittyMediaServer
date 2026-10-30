package org.bluray.ui;

/** Drawn frame by frame by the application. */
public class SyncFrameAccurateAnimation extends FrameAccurateAnimation {
    private final java.util.List<Object> listeners = new java.util.ArrayList<Object>();
    public SyncFrameAccurateAnimation(java.awt.Dimension size, int buffers, AnimationParameters p) { super(size, new java.awt.Image[0], p); }
    public java.awt.Graphics2D startFrame(int n) { return (java.awt.Graphics2D) new java.awt.image.BufferedImage(Math.max(1, getWidth()), Math.max(1, getHeight()), java.awt.image.BufferedImage.TYPE_INT_ARGB).getGraphics(); }
    public void finishFrame() { repaint(); }
}
