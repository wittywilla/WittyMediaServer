package javax.tv.graphics;
public class AlphaColor extends java.awt.Color {
    public AlphaColor(float r, float g, float b, float a) { super(r, g, b, a); }
    public AlphaColor(int r, int g, int b, int a) { super(r, g, b, a); }
    public AlphaColor(int rgba, boolean has) { super(rgba, has); }
    public AlphaColor(java.awt.Color c) { super(c.getRGB(), true); }
}
