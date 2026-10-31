package org.havi.ui;
public class HScreen {
    private static final HScreen it = new HScreen();
    private static final HGraphicsDevice g = new HGraphicsDevice();
    private static final HVideoDevice v = new HVideoDevice();
    private static final HBackgroundDevice b = new HBackgroundDevice();
    private HScreen() {}
    public static HScreen[] getHScreens() { return new HScreen[] {it}; }
    public static HScreen getDefaultHScreen() { return it; }
    public HVideoDevice[] getHVideoDevices() { return new HVideoDevice[] {v}; }
    public HGraphicsDevice[] getHGraphicsDevices() { return new HGraphicsDevice[] {g}; }
    public HBackgroundDevice[] getHBackgroundDevices() { return new HBackgroundDevice[] {b}; }
    public HVideoDevice getDefaultHVideoDevice() { return v; }
    public HGraphicsDevice getDefaultHGraphicsDevice() { return g; }
    public HBackgroundDevice getDefaultHBackgroundDevice() { return b; }
    public HScreenConfiguration[] getCoherentScreenConfigurations(HScreenConfigTemplate[] t) {
        if (t == null) return null;
        HScreenConfiguration[] o = new HScreenConfiguration[t.length];
        for (int i = 0; i < t.length; i++) {
            if (t[i] instanceof HVideoConfigTemplate) o[i] = v.getCurrentConfiguration();
            else if (t[i] instanceof HBackgroundConfigTemplate) o[i] = b.getCurrentConfiguration();
            else o[i] = g.getCurrentConfiguration();
        }
        return o;
    }
    public boolean setCoherentScreenConfigurations(HScreenConfiguration[] c) throws SecurityException, HPermissionDeniedException, HConfigurationException { return c != null; }
    public HVideoConfiguration getBestConfiguration(HVideoConfigTemplate[] t) { return v.getDefaultConfiguration(); }
    public HGraphicsConfiguration getBestConfiguration(HGraphicsConfigTemplate[] t) { return g.getDefaultConfiguration(); }
    public HBackgroundConfiguration getBestConfiguration(HBackgroundConfigTemplate[] t) { return b.getDefaultConfiguration(); }
}
