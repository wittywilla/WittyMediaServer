package org.havi.ui;
public class HBackgroundConfiguration extends HScreenConfiguration {
    private java.awt.Color color = java.awt.Color.black;
    protected HBackgroundConfiguration() {}
    public HBackgroundDevice getDevice() { return HScreen.getDefaultHScreen().getDefaultHBackgroundDevice(); }
    public HBackgroundConfigTemplate getConfigTemplate() { return new HBackgroundConfigTemplate(); }
    public java.awt.Color getColor() { return color; }
    public void setColor(java.awt.Color c) throws HPermissionDeniedException, HConfigurationException {
        if (c == null) throw new NullPointerException();
        color = c;
        HBackLayer.color(c);
    }
}
