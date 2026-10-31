package org.havi.ui;

import java.awt.Color;
import java.awt.Graphics2D;
import java.awt.Image;
import witty.bdj.Apps;
import witty.bdj.Host;
import witty.bdj.Plane;
import witty.bdj.Playback;

/** The background device: under everything on the plane, and not over the video, which it would be behind on a real player. */
final class HBackLayer implements Plane.Layer {
    private static final HBackLayer it = new HBackLayer();
    private static boolean added;
    private volatile Color fill;
    private volatile Image pic;
    private volatile HScreenRectangle area;
    private volatile Apps.App owner;
    private HBackLayer() {}
    static synchronized void color(Color c) { it.fill = c; it.pic = null; show(); }
    static synchronized void picture(Image p, HScreenRectangle r) { it.pic = p; it.area = r; show(); }
    private static void show() {
        it.owner = Apps.owner();
        if (!added) { added = true; Plane.addBottom(it); }
        Plane.invalidate();
    }
    public boolean visibleLayer() {
        Apps.App a = owner;
        return (fill != null || pic != null) && !Playback.isRunning() && (a == null || !a.ended());
    }
    public void paintLayer(Graphics2D g) {
        int W = Host.width, H = Host.height;
        Color c = fill;
        if (c != null) { g.setColor(c); g.fillRect(0, 0, W, H); }
        Image p = pic;
        if (p == null) return;
        HScreenRectangle r = area;
        if (r == null) g.drawImage(p, 0, 0, W, H, null);
        else g.drawImage(p, Math.round(r.x * W), Math.round(r.y * H), Math.round(r.width * W), Math.round(r.height * H), null);
    }
}
