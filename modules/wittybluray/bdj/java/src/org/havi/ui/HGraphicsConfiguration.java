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
    public java.awt.Font[] getAllFonts() { return new java.awt.Font[] {new java.awt.Font("SansSerif", java.awt.Font.PLAIN, 24), new java.awt.Font("Serif", java.awt.Font.PLAIN, 24), new java.awt.Font("Monospaced", java.awt.Font.PLAIN, 24)}; }
    public HScreenRectangle getComponentHScreenRectangle(java.awt.Component c) {
        if (c == null) return null;
        int x = 0, y = 0;
        for (java.awt.Component p = c; p != null; p = p.getParent()) {
            x += p.getX();
            y += p.getY();
            if (p instanceof HScene) break;
        }
        float W = witty.bdj.Host.width, H = witty.bdj.Host.height;
        return new HScreenRectangle(x / W, y / H, c.getWidth() / W, c.getHeight() / H);
    }
    public java.awt.Rectangle getPixelCoordinatesHScreenRectangle(HScreenRectangle r, java.awt.Container c) {
        if (r == null) return null;
        int W = witty.bdj.Host.width, H = witty.bdj.Host.height;
        java.awt.Rectangle o = new java.awt.Rectangle(Math.round(r.x * W), Math.round(r.y * H), Math.round(r.width * W), Math.round(r.height * H));
        for (java.awt.Component p = c; p != null; p = p.getParent()) {
            o.x -= p.getX();
            o.y -= p.getY();
            if (p instanceof HScene) break;
        }
        return o;
    }
}
