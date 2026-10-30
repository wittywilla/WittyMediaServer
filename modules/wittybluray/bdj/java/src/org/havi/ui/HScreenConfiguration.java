package org.havi.ui;
public abstract class HScreenConfiguration {
    HScreenConfiguration() {}
    public java.awt.Dimension getPixelResolution() { return new java.awt.Dimension(witty.bdj.Host.width, witty.bdj.Host.height); }
    public HScreenRectangle getScreenArea() { return new HScreenRectangle(0, 0, 1, 1); }
    public java.awt.Dimension getPixelAspectRatio() { return new java.awt.Dimension(1, 1); }
    public java.awt.Dimension getScreenAspectRatio() { return new java.awt.Dimension(16, 9); }
    public boolean getFlickerFilter() { return false; }
    public boolean getInterlaced() { return false; }
    public java.awt.Point convertTo(HScreenConfiguration c, java.awt.Point p) { return p; }
    public java.awt.Rectangle convertTo(HScreenConfiguration c, java.awt.Rectangle r) { return r; }
    public java.awt.Point getOffset(HScreenConfiguration c) { return new java.awt.Point(0, 0); }
}
