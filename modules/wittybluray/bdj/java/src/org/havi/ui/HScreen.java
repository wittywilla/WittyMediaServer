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
    public HScreenConfiguration[] getCoherentScreenConfigurations(HScreenConfigTemplate[] t) { return new HScreenConfiguration[0]; }
    public boolean setCoherentScreenConfigurations(HScreenConfiguration[] c) { return true; }
    public HVideoConfiguration getBestConfiguration(HVideoConfigTemplate[] t) { return v.getDefaultConfiguration(); }
    public HGraphicsConfiguration getBestConfiguration(HGraphicsConfigTemplate[] t) { return g.getDefaultConfiguration(); }
    public HBackgroundConfiguration getBestConfiguration(HBackgroundConfigTemplate[] t) { return b.getDefaultConfiguration(); }
}
