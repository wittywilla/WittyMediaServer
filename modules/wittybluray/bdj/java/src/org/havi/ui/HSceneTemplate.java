package org.havi.ui;
public class HSceneTemplate {
    public static final int REQUIRED = 1, PREFERRED = 2, UNNECESSARY = 3;
    public static final int GRAPHICS_CONFIGURATION = 0, SCENE_PIXEL_RECTANGLE = 1, SCENE_PIXEL_LOCATION = 2, SCENE_PIXEL_DIMENSION = 3, SCENE_SCREEN_RECTANGLE = 4, SCENE_SCREEN_LOCATION = 5, SCENE_SCREEN_DIMENSION = 6;
    public static final java.awt.Dimension LARGEST_PIXEL_DIMENSION = new java.awt.Dimension(1920, 1080);
    private final java.util.Map<Integer, Object> v = new java.util.HashMap<Integer, Object>();
    public void setPreference(int k, Object o, int p) { v.put(k, o); }
    public Object getPreferenceObject(int k) { return v.get(k); }
    public int getPreferencePriority(int k) { return v.containsKey(k) ? PREFERRED : UNNECESSARY; }
}
