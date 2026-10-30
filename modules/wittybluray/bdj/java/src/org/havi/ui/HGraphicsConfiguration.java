package org.havi.ui;
public class HGraphicsConfiguration extends HScreenConfiguration {
    protected HGraphicsConfiguration() {}
    public HGraphicsDevice getDevice() { return HScreen.getDefaultHScreen().getDefaultHGraphicsDevice(); }
    public HGraphicsConfigTemplate getConfigTemplate() { return new HGraphicsConfigTemplate(); }
    public java.awt.Color getPunchThroughToBackgroundColor(int percentage) { return new org.dvb.ui.DVBColor(0, 0, 0, 0); }
    public java.awt.Color getPunchThroughToBackgroundColor(java.awt.Color c, int percentage) { return new org.dvb.ui.DVBColor(0, 0, 0, 0); }
    public java.awt.Color getPunchThroughToBackgroundColor(int percentage, HVideoDevice v) { return new org.dvb.ui.DVBColor(0, 0, 0, 0); }
    public java.awt.Color getPunchThroughToBackgroundColor(java.awt.Color c, int percentage, HVideoDevice v) { return new org.dvb.ui.DVBColor(0, 0, 0, 0); }
    public void dispose(java.awt.Color c) {}
    public java.awt.Image getCompatibleImage(java.awt.Image i, HImageHints h) { return i; }
    public java.awt.Font[] getAllFonts() { return new java.awt.Font[0]; }
}
