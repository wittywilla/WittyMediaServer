package org.havi.ui;
public class HStaticRange extends HVisible implements HNoInputPreferred, HOrientable {
    public static final int SLIDER_BEHAVIOR = 0, SCROLLBAR_BEHAVIOR = 1;
    private static HRangeLook look = new HRangeLook();
    public static void setDefaultLook(HRangeLook l) { look = l; }
    public static HRangeLook getDefaultLook() { return look; }
    private int orientation = ORIENT_LEFT_TO_RIGHT, min, max = 100, value, thumbMin, thumbMax, behavior = SLIDER_BEHAVIOR;
    public HStaticRange() { super(look); }
    public HStaticRange(int o, int mn, int mx, int v) { this(); orientation = o; setRange(mn, mx); setValue(v); }
    public HStaticRange(int o, int mn, int mx, int v, int x, int y, int w, int h) { this(o, mn, mx, v); setBounds(x, y, w, h); }
    public int getOrientation() { return orientation; }
    public void setOrientation(int o) { if (o >= ORIENT_LEFT_TO_RIGHT && o <= ORIENT_TOP_TO_BOTTOM) { orientation = o; changed(ORIENTATION_CHANGE, Integer.valueOf(o)); } }
    public boolean setRange(int mn, int mx) {
        if (mn >= mx) return false;
        min = mn;
        max = mx;
        setValue(value);
        changed(MIN_MAX_CHANGE, null);
        return true;
    }
    public int getMinValue() { return min; }
    public int getMaxValue() { return max; }
    public void setValue(int v) {
        int low = min + thumbMin, high = max - thumbMax;
        value = Math.max(low, Math.min(high, v));
        changed(ADJUSTMENT_VALUE_CHANGE, null);
    }
    public int getValue() { return value; }
    public void setThumbOffsets(int a, int b) {
        if (a < 0 || b < 0 || a + b > max - min) return;
        thumbMin = a;
        thumbMax = b;
        setValue(value);
        changed(THUMB_OFFSETS_CHANGE, null);
    }
    public int getThumbMinOffset() { return thumbMin; }
    public int getThumbMaxOffset() { return thumbMax; }
    public void setBehavior(int b) { if (b == SLIDER_BEHAVIOR || b == SCROLLBAR_BEHAVIOR) { behavior = b; repaint(); } }
    public int getBehavior() { return behavior; }
}
