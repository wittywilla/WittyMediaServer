package org.havi.ui;
public abstract class HScreenConfigTemplate {
    public static final int REQUIRED = 1, PREFERRED = 2, UNNECESSARY = 3, REQUIRED_NOT = 4, PREFERRED_NOT = 5, DONT_CARE = 3;
    public static final int ZERO_BACKGROUND_IMPACT = 1, ZERO_GRAPHICS_IMPACT = 2, ZERO_VIDEO_IMPACT = 3, INTERLACED_DISPLAY = 4, FLICKER_FILTERING = 5, VIDEO_GRAPHICS_PIXEL_ALIGNED = 6, PIXEL_ASPECT_RATIO = 7, PIXEL_RESOLUTION = 8, SCREEN_RECTANGLE = 9;
    private final java.util.Map<Integer, Object> v = new java.util.HashMap<Integer, Object>();
    public void setPreference(int k, int p) { v.put(k, p); }
    public void setPreference(int k, Object o, int p) { v.put(k, o); }
    public int getPreferencePriority(int k) { Object o = v.get(k); return o instanceof Integer ? (Integer) o : DONT_CARE; }
    public Object getPreferenceObject(int k) { return v.get(k); }
    public boolean isConfigSupported(HScreenConfiguration c) { return true; }
}
