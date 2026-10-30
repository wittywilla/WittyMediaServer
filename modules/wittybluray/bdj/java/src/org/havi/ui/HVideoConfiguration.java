package org.havi.ui;
public class HVideoConfiguration extends HScreenConfiguration {
    protected HVideoConfiguration() {}
    public HVideoDevice getDevice() { return HScreen.getDefaultHScreen().getDefaultHVideoDevice(); }
    public HVideoConfigTemplate getConfigTemplate() { return new HVideoConfigTemplate(); }
}
