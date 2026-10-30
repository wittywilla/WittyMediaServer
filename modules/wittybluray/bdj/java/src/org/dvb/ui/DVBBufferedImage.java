package org.dvb.ui;

import java.awt.Graphics;
import java.awt.Image;
import java.awt.image.BufferedImage;
import java.awt.image.ImageObserver;
import java.awt.image.ImageProducer;

public class DVBBufferedImage extends Image {
    public static final int TYPE_ADVANCED = 20, TYPE_BASE = 21;
    private BufferedImage img;
    public DVBBufferedImage(int w, int h) { img = new BufferedImage(w, h, BufferedImage.TYPE_INT_ARGB); }
    public DVBBufferedImage(int w, int h, int type) { this(w, h); }
    public void dispose() { img = null; }
    public void flush() {}
    public Graphics getGraphics() { return createGraphics(); }
    public DVBGraphics createGraphics() { return new DVBGraphicsImpl(img.createGraphics()); }
    public int getHeight() { return img.getHeight(); }
    public int getWidth() { return img.getWidth(); }
    public int getHeight(ImageObserver o) { return img.getHeight(); }
    public int getWidth(ImageObserver o) { return img.getWidth(); }
    public Object getProperty(String n, ImageObserver o) { return img.getProperty(n, o); }
    public ImageProducer getSource() { return img.getSource(); }
    public int getRGB(int x, int y) { return img.getRGB(x, y); }
    public int[] getRGB(int x, int y, int w, int h, int[] a, int o, int s) { return img.getRGB(x, y, w, h, a, o, s); }
    public void setRGB(int x, int y, int v) { img.setRGB(x, y, v); }
    public void setRGB(int x, int y, int w, int h, int[] a, int o, int s) { img.setRGB(x, y, w, h, a, o, s); }
    public Image getSubimage(int x, int y, int w, int h) throws DVBRasterFormatException { return img.getSubimage(x, y, w, h); }
    public Image getImage() { return img; }
    public int getType() { return TYPE_ADVANCED; }
}
