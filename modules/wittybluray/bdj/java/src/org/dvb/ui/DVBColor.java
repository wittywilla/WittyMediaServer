package org.dvb.ui;
public class DVBColor extends java.awt.Color {
    public DVBColor(float r, float g, float b, float a) { super(r, g, b, a); }
    public DVBColor(int r, int g, int b, int a) { super(r, g, b, a); }
    public DVBColor(int rgba, boolean has) { super(rgba, has); }
    public DVBColor(java.awt.Color c) { super(c.getRGB(), true); }
}
