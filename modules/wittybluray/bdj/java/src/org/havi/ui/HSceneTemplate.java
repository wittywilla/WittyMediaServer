package org.havi.ui;

import java.awt.Dimension;
import java.awt.Point;
import java.awt.Rectangle;
import witty.bdj.Host;

public class HSceneTemplate {
    public static final int REQUIRED = 1, PREFERRED = 2, UNNECESSARY = 3;
    public static final int GRAPHICS_CONFIGURATION = 0, SCENE_PIXEL_DIMENSION = 1, SCENE_PIXEL_LOCATION = 2, SCENE_SCREEN_DIMENSION = 4, SCENE_SCREEN_LOCATION = 8;
    public static final Dimension LARGEST_PIXEL_DIMENSION = new Dimension(-1, -1);
    private final java.util.Map<Integer, Object> v = new java.util.concurrent.ConcurrentHashMap<Integer, Object>();
    private final java.util.Map<Integer, Integer> p = new java.util.concurrent.ConcurrentHashMap<Integer, Integer>();
    public HSceneTemplate() {}
    public void setPreference(int k, Object o, int priority) { p.put(k, priority); if (o == null) v.remove(k); else v.put(k, o); }
    public Object getPreferenceObject(int k) { return v.get(k); }
    public int getPreferencePriority(int k) { Integer i = p.get(k); return i != null ? i : UNNECESSARY; }
    /** Where a scene made to this template goes: inside the plane, whatever was asked for. The kind of each value says what it means. */
    final Rectangle bounds() {
        int W = Host.width, H = Host.height, x = 0, y = 0, w = W, h = H;
        for (java.util.Map.Entry<Integer, Object> e : v.entrySet()) {
            if (getPreferencePriority(e.getKey()) == UNNECESSARY) continue;
            Object o = e.getValue();
            if (o instanceof HScreenDimension) { w = Math.round(((HScreenDimension) o).width * W); h = Math.round(((HScreenDimension) o).height * H); }
            else if (o instanceof Dimension && ((Dimension) o).width > 0 && ((Dimension) o).height > 0) { w = ((Dimension) o).width; h = ((Dimension) o).height; }
            else if (o instanceof HScreenPoint) { x = Math.round(((HScreenPoint) o).x * W); y = Math.round(((HScreenPoint) o).y * H); }
            else if (o instanceof Point) { x = ((Point) o).x; y = ((Point) o).y; }
        }
        x = Math.max(0, Math.min(W - 1, x));
        y = Math.max(0, Math.min(H - 1, y));
        return new Rectangle(x, y, Math.max(1, Math.min(W - x, w)), Math.max(1, Math.min(H - y, h)));
    }
}
