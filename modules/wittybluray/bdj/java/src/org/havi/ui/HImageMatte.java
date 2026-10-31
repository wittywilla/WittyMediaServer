package org.havi.ui;
public class HImageMatte implements HMatte {
    private java.awt.Image data;
    private java.awt.Point offset = new java.awt.Point(0, 0);
    public HImageMatte() {}
    public HImageMatte(java.awt.Image d) { data = d; }
    public void setMatteData(java.awt.Image d) { data = d; witty.bdj.Plane.invalidate(); }
    public java.awt.Image getMatteData() { return data; }
    public void setOffset(java.awt.Point p) { offset = p == null ? new java.awt.Point(0, 0) : p; witty.bdj.Plane.invalidate(); }
    public java.awt.Point getOffset() { return offset; }
}
