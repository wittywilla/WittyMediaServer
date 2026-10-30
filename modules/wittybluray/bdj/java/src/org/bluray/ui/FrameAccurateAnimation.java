package org.bluray.ui;

import java.awt.Graphics;
import java.awt.Image;

/** An animation synchronised to the video. The picture runs at the plane's rate, against the video's clock. */
public class FrameAccurateAnimation extends org.havi.ui.HComponent {
    public static final int THREAD_PRIORITY = 5;
    protected final Image[] images;
    protected final AnimationParameters params;
    private volatile boolean running;
    private volatile int frame;
    public FrameAccurateAnimation(java.awt.Rectangle[] bounds, java.awt.Dimension size, AnimationParameters p) { images = new Image[0]; params = p; setBounds(0, 0, size.width, size.height); }
    public FrameAccurateAnimation(java.awt.Dimension size, Image[] i, AnimationParameters p) { images = i; params = p; setSize(size); }
    public void start() {
        running = true;
        Thread t = new Thread(new Runnable() { public void run() {
            while (running) {
                frame++;
                repaint();
                try { Thread.sleep(1000 / witty.bdj.Plane.FPS); } catch (InterruptedException e) { return; }
            }
        } });
        t.setDaemon(true);
        t.start();
    }
    public void stop() { running = false; }
    public boolean isAnimated() { return running; }
    public void destroy() { stop(); }
    public void prepareImages() {}
    public int getCurrentFrame() { return frame; }
    public void paint(Graphics g) {
        if (images.length > 0 && images[frame % images.length] != null) g.drawImage(images[frame % images.length], 0, 0, this);
    }
}
