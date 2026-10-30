package org.havi.ui;
public class HBackgroundConfiguration extends HScreenConfiguration {
    protected HBackgroundConfiguration() {}
    public HBackgroundDevice getDevice() { return HScreen.getDefaultHScreen().getDefaultHBackgroundDevice(); }
    public HBackgroundConfigTemplate getConfigTemplate() { return new HBackgroundConfigTemplate(); }
    public java.awt.Color getColor() { return java.awt.Color.black; }
    public void setColor(java.awt.Color c) {}
}
