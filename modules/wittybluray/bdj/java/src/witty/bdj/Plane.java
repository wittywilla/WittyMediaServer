package witty.bdj;

import java.awt.AlphaComposite;
import java.awt.Graphics2D;
import java.awt.Rectangle;
import java.awt.image.BufferedImage;
import java.awt.image.DataBufferInt;
import java.util.List;
import java.util.concurrent.CopyOnWriteArrayList;

/**
 * The graphics plane: every scene is painted into one picture at most Plane.FPS
 * times a second when something asked to be repainted, and only the rectangle
 * that really changed goes to the server. A small machine is spared most of the
 * work of a menu that sits still.
 */
public final class Plane {
    private Plane() {}

    public static final int FPS = 12;
    /** A scene the plane paints. */
    public interface Layer {
        void paintLayer(Graphics2D g);
        boolean visibleLayer();
    }

    private static final List<Layer> layers = new CopyOnWriteArrayList<Layer>();
    private static volatile boolean dirty = true;
    private static BufferedImage front, back;
    private static int[] frontPix, backPix;

    public static void add(Layer l) {
        layers.add(l);
        invalidate();
    }

    public static void remove(Layer l) {
        layers.remove(l);
        invalidate();
    }

    public static void invalidate() { dirty = true; }

    /** A drawing surface as big as the plane, for scenes that draw directly. */
    public static BufferedImage makeImage(int w, int h) {
        return new BufferedImage(Math.max(1, w), Math.max(1, h), BufferedImage.TYPE_INT_ARGB);
    }

    static void start() {
        front = makeImage(Host.width, Host.height);
        back = makeImage(Host.width, Host.height);
        frontPix = ((DataBufferInt) front.getRaster().getDataBuffer()).getData();
        backPix = ((DataBufferInt) back.getRaster().getDataBuffer()).getData();
        Thread t = new Thread(new Runnable() {
            public void run() {
                long gap = 1000 / FPS;
                for (;;) {
                    try {
                        Thread.sleep(gap);
                        if (dirty) {
                            dirty = false;
                            paint();
                        }
                    } catch (InterruptedException e) {
                        // Disc code can reach this thread; stopping it would blank the picture for good.
                    } catch (Throwable e) {
                        Host.log("paint: " + e); // a disc's paint method can throw anything
                    }
                }
            }
        }, "bdj-plane");
        t.setDaemon(true);
        t.start();
    }

    private static void paint() {
        Graphics2D g = back.createGraphics();
        g.setComposite(AlphaComposite.Clear);
        g.fillRect(0, 0, Host.width, Host.height);
        g.setComposite(AlphaComposite.SrcOver);
        for (Layer l : layers) {
            if (!l.visibleLayer()) continue;
            Graphics2D c = (Graphics2D) g.create();
            try {
                l.paintLayer(c);
            } catch (Throwable e) {
                Host.log("scene: " + e);
            } finally {
                c.dispose();
            }
        }
        g.dispose();
        int w = Host.width, h = Host.height;
        int top = -1, bottom = -1, left = w, right = -1;
        for (int y = 0; y < h; y++) {
            int o = y * w;
            boolean any = false;
            for (int x = 0; x < w; x++) {
                if (backPix[o + x] != frontPix[o + x]) {
                    if (x < left) left = x;
                    if (x > right) right = x;
                    any = true;
                }
            }
            if (any) {
                if (top < 0) top = y;
                bottom = y;
            }
        }
        if (top < 0) return;
        Host.sendFrame(left, top, right - left + 1, bottom - top + 1, backPix, top * w + left, w);
        Host.send("flush");
        System.arraycopy(backPix, 0, frontPix, 0, backPix.length);
    }
}
