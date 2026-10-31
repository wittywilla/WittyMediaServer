package org.havi.ui;
public class HVideoDevice extends HScreenDevice {
    public static final HVideoConfiguration NOT_CONTRIBUTING = null;
    private final HVideoConfiguration c = new HVideoConfiguration();
    protected HVideoDevice() {}
    public HVideoConfiguration[] getConfigurations() { return new HVideoConfiguration[] {c}; }
    public HVideoConfiguration getDefaultConfiguration() { return c; }
    public HVideoConfiguration getCurrentConfiguration() { return c; }
    public boolean setVideoConfiguration(HVideoConfiguration n) throws SecurityException, HPermissionDeniedException, HConfigurationException { return n != null; }
    public HVideoConfiguration getBestConfiguration(HVideoConfigTemplate t) { return c; }
    public HVideoConfiguration getBestConfiguration(HVideoConfigTemplate[] t) { return c; }
    public Object getVideoSource() { return null; }
    public Object getVideoController() { return null; }
}
