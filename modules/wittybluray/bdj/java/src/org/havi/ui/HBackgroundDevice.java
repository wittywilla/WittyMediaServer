package org.havi.ui;
public class HBackgroundDevice extends HScreenDevice {
    private final HStillImageBackgroundConfiguration c = new HStillImageBackgroundConfiguration();
    protected HBackgroundDevice() {}
    public HBackgroundConfiguration[] getConfigurations() { return new HBackgroundConfiguration[] {c}; }
    public HBackgroundConfiguration getDefaultConfiguration() { return c; }
    public HBackgroundConfiguration getCurrentConfiguration() { return c; }
    public boolean setBackgroundConfiguration(HBackgroundConfiguration n) { return true; }
    public HBackgroundConfiguration getBestConfiguration(HBackgroundConfigTemplate t) { return c; }
    public HBackgroundConfiguration getBestConfiguration(HBackgroundConfigTemplate[] t) { return c; }
}
