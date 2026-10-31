package org.dvb.ui;

import java.awt.*;
import java.awt.font.FontRenderContext;
import java.awt.font.GlyphVector;
import java.awt.geom.AffineTransform;
import java.awt.image.BufferedImage;
import java.awt.image.BufferedImageOp;
import java.awt.image.ImageObserver;
import java.awt.image.RenderedImage;
import java.awt.image.renderable.RenderableImage;
import java.text.AttributedCharacterIterator;
import java.util.Map;

/** DVBGraphics over an ordinary Java 2D surface. */
public final class DVBGraphicsImpl extends DVBGraphics {
    private final Graphics2D g;
    private DVBAlphaComposite dvb = DVBAlphaComposite.SrcOver;
    public DVBGraphicsImpl(Graphics2D g) { this.g = g; }
    private static Image u(Image i) { return i instanceof DVBBufferedImage ? ((DVBBufferedImage) i).getImage() : i; }
    public int[] getAvailableCompositeRules() { return new int[] {1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}; }
    public DVBAlphaComposite getDVBComposite() { return dvb; }
    public void setDVBComposite(DVBAlphaComposite c) { dvb = c; g.setComposite(c.toAWT()); }
    public int getType() { return BufferedImage.TYPE_INT_ARGB; }
    public Color getBestColorMatch(Color c) { return c; }
    public void setComposite(Composite c) {
        if (c instanceof DVBAlphaComposite) setDVBComposite((DVBAlphaComposite) c);
        else g.setComposite(c);
    }
    public void draw(Shape s) { g.draw(s); }
    public boolean drawImage(Image i, AffineTransform t, ImageObserver o) { return g.drawImage(u(i), t, o); }
    public void drawImage(BufferedImage i, BufferedImageOp op, int x, int y) { g.drawImage(i, op, x, y); }
    public void drawRenderedImage(RenderedImage i, AffineTransform t) { g.drawRenderedImage(i, t); }
    public void drawRenderableImage(RenderableImage i, AffineTransform t) { g.drawRenderableImage(i, t); }
    public void drawString(String s, int x, int y) { g.drawString(s, x, y); }
    public void drawString(String s, float x, float y) { g.drawString(s, x, y); }
    public void drawString(AttributedCharacterIterator i, int x, int y) { g.drawString(i, x, y); }
    public void drawString(AttributedCharacterIterator i, float x, float y) { g.drawString(i, x, y); }
    public void drawGlyphVector(GlyphVector v, float x, float y) { g.drawGlyphVector(v, x, y); }
    public void fill(Shape s) { g.fill(s); }
    public boolean hit(Rectangle r, Shape s, boolean b) { return g.hit(r, s, b); }
    public GraphicsConfiguration getDeviceConfiguration() { return g.getDeviceConfiguration(); }
    public void setPaint(Paint p) { g.setPaint(p); }
    public void setStroke(Stroke s) { g.setStroke(s); }
    public void setRenderingHint(RenderingHints.Key k, Object v) { g.setRenderingHint(k, v); }
    public Object getRenderingHint(RenderingHints.Key k) { return g.getRenderingHint(k); }
    public void setRenderingHints(Map<?, ?> h) { g.setRenderingHints(h); }
    public void addRenderingHints(Map<?, ?> h) { g.addRenderingHints(h); }
    public RenderingHints getRenderingHints() { return g.getRenderingHints(); }
    public void translate(int x, int y) { g.translate(x, y); }
    public void translate(double x, double y) { g.translate(x, y); }
    public void rotate(double t) { g.rotate(t); }
    public void rotate(double t, double x, double y) { g.rotate(t, x, y); }
    public void scale(double x, double y) { g.scale(x, y); }
    public void shear(double x, double y) { g.shear(x, y); }
    public void transform(AffineTransform t) { g.transform(t); }
    public void setTransform(AffineTransform t) { g.setTransform(t); }
    public AffineTransform getTransform() { return g.getTransform(); }
    public Paint getPaint() { return g.getPaint(); }
    public Composite getComposite() { return g.getComposite(); }
    public void setBackground(Color c) { g.setBackground(c); }
    public Color getBackground() { return g.getBackground(); }
    public Stroke getStroke() { return g.getStroke(); }
    public void clip(Shape s) { g.clip(s); }
    public FontRenderContext getFontRenderContext() { return g.getFontRenderContext(); }
    public Graphics create() { DVBGraphicsImpl c = new DVBGraphicsImpl((Graphics2D) g.create()); c.dvb = dvb; return c; }
    public Color getColor() { return g.getColor(); }
    public void setColor(Color c) { g.setColor(c); }
    public void setPaintMode() { g.setPaintMode(); }
    public void setXORMode(Color c) { g.setXORMode(c); }
    public Font getFont() { return g.getFont(); }
    public void setFont(Font f) { g.setFont(f); }
    public FontMetrics getFontMetrics(Font f) { return g.getFontMetrics(f); }
    public Rectangle getClipBounds() { return g.getClipBounds(); }
    public void clipRect(int x, int y, int w, int h) { g.clipRect(x, y, w, h); }
    public void setClip(int x, int y, int w, int h) { g.setClip(x, y, w, h); }
    public Shape getClip() { return g.getClip(); }
    public void setClip(Shape s) { g.setClip(s); }
    public void copyArea(int x, int y, int w, int h, int dx, int dy) { g.copyArea(x, y, w, h, dx, dy); }
    public void drawLine(int a, int b, int c, int d) { g.drawLine(a, b, c, d); }
    public void fillRect(int x, int y, int w, int h) { g.fillRect(x, y, w, h); }
    public void clearRect(int x, int y, int w, int h) { g.clearRect(x, y, w, h); }
    public void drawRoundRect(int x, int y, int w, int h, int a, int b) { g.drawRoundRect(x, y, w, h, a, b); }
    public void fillRoundRect(int x, int y, int w, int h, int a, int b) { g.fillRoundRect(x, y, w, h, a, b); }
    public void drawOval(int x, int y, int w, int h) { g.drawOval(x, y, w, h); }
    public void fillOval(int x, int y, int w, int h) { g.fillOval(x, y, w, h); }
    public void drawArc(int x, int y, int w, int h, int a, int b) { g.drawArc(x, y, w, h, a, b); }
    public void fillArc(int x, int y, int w, int h, int a, int b) { g.fillArc(x, y, w, h, a, b); }
    public void drawPolyline(int[] x, int[] y, int n) { g.drawPolyline(x, y, n); }
    public void drawPolygon(int[] x, int[] y, int n) { g.drawPolygon(x, y, n); }
    public void fillPolygon(int[] x, int[] y, int n) { g.fillPolygon(x, y, n); }
    public boolean drawImage(Image i, int x, int y, ImageObserver o) { return g.drawImage(u(i), x, y, o); }
    public boolean drawImage(Image i, int x, int y, int w, int h, ImageObserver o) { return g.drawImage(u(i), x, y, w, h, o); }
    public boolean drawImage(Image i, int x, int y, Color c, ImageObserver o) { return g.drawImage(u(i), x, y, c, o); }
    public boolean drawImage(Image i, int x, int y, int w, int h, Color c, ImageObserver o) { return g.drawImage(u(i), x, y, w, h, c, o); }
    public boolean drawImage(Image i, int a, int b, int c, int d, int e, int f, int h, int j, ImageObserver o) { return g.drawImage(u(i), a, b, c, d, e, f, h, j, o); }
    public boolean drawImage(Image i, int a, int b, int c, int d, int e, int f, int h, int j, Color k, ImageObserver o) { return g.drawImage(u(i), a, b, c, d, e, f, h, j, k, o); }
    public void dispose() { g.dispose(); }
}
