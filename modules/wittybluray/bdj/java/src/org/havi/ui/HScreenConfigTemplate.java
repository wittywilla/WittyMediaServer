package org.havi.ui;
public abstract class HScreenConfigTemplate {
    public static final int REQUIRED = 1, PREFERRED = 2, UNNECESSARY = 3, PREFERRED_NOT = 4, REQUIRED_NOT = 5;
    public static final int ZERO_BACKGROUND_IMPACT = 1, ZERO_GRAPHICS_IMPACT = 2, ZERO_VIDEO_IMPACT = 3, INTERLACED_DISPLAY = 4, FLICKER_FILTERING = 5, VIDEO_GRAPHICS_PIXEL_ALIGNED = 6, PIXEL_ASPECT_RATIO = 7, PIXEL_RESOLUTION = 8, SCREEN_RECTANGLE = 9;
    private final java.util.Map<Integer, Integer> priorities = new java.util.concurrent.ConcurrentHashMap<Integer, Integer>();
    private final java.util.Map<Integer, Object> objects = new java.util.concurrent.ConcurrentHashMap<Integer, Object>();
    public void setPreference(int k, int p) { priorities.put(k, p); }
    public void setPreference(int k, Object o, int p) { priorities.put(k, p); if (o == null) objects.remove(k); else objects.put(k, o); }
    public int getPreferencePriority(int k) { Integer p = priorities.get(k); return p != null ? p : UNNECESSARY; }
    public Object getPreferenceObject(int k) { return objects.get(k); }
    public boolean isConfigSupported(HScreenConfiguration c) { return c != null; }
}
