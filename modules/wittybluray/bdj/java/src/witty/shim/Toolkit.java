package witty.shim;

import java.awt.Dimension;
import java.awt.Font;
import java.awt.FontMetrics;
import java.awt.Image;
import java.awt.image.BufferedImage;
import java.awt.image.ColorModel;
import java.awt.image.ImageObserver;
import java.awt.image.ImageProducer;
import java.io.ByteArrayInputStream;
import java.io.IOException;
import witty.bdj.Host;

/** Loads pictures at once, from the disc's own files, for a player with no screen. */
public class Toolkit {
    private static final Toolkit it = new Toolkit();
    private static final BufferedImage scratch = new BufferedImage(1, 1, BufferedImage.TYPE_INT_ARGB);
    protected Toolkit() {}
    public static Toolkit getDefaultToolkit() { return it; }
    public Image getImage(String path) { return createImage(path); }
    public Image getImage(URL url) { return createImage(url); }
    public Image createImage(String path) {
        try {
            java.io.InputStream in = new FileInputStream(path);
            try {
                return decode(in);
            } finally {
                in.close();
            }
        } catch (IOException e) {
            return empty();
        }
    }
    public Image createImage(URL url) {
        try {
            return decode(new ByteArrayInputStream(url.bytes()));
        } catch (IOException e) {
            return empty();
        }
    }
    public Image createImage(byte[] b) { return createImage(b, 0, b.length); }
    public Image createImage(byte[] b, int o, int l) {
        try {
            return decode(new ByteArrayInputStream(b, o, l));
        } catch (IOException e) {
            return empty();
        }
    }
    public Image createImage(ImageProducer p) { return java.awt.Toolkit.getDefaultToolkit().createImage(p); }
    private static Image empty() { return new BufferedImage(1, 1, BufferedImage.TYPE_INT_ARGB); }
    private static Image decode(java.io.InputStream in) throws IOException {
        BufferedImage i = javax.imageio.ImageIO.read(in);
        return i == null ? empty() : i;
    }
    public FontMetrics getFontMetrics(Font f) { java.awt.Graphics g = scratch.getGraphics(); try { return g.getFontMetrics(f); } finally { g.dispose(); } }
    public Dimension getScreenSize() { return new Dimension(Host.width, Host.height); }
    public int getScreenResolution() { return 96; }
    public ColorModel getColorModel() { return ColorModel.getRGBdefault(); }
    public void sync() {}
    public void beep() {}
    public boolean prepareImage(Image i, int w, int h, ImageObserver o) { return true; }
    public int checkImage(Image i, int w, int h, ImageObserver o) { return ImageObserver.ALLBITS | ImageObserver.WIDTH | ImageObserver.HEIGHT; }
    public String[] getFontList() { return new String[] {"SansSerif", "Serif", "Monospaced", "Dialog"}; }
}
