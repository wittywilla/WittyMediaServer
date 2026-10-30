package javax.tv.media;
public class AWTVideoSize {
    private final java.awt.Rectangle s, d;
    public AWTVideoSize(java.awt.Rectangle source, java.awt.Rectangle dest) { s = source; d = dest; }
    public java.awt.Rectangle getSource() { return s; }
    public java.awt.Rectangle getDestination() { return d; }
    public float getXScale() { return d.width / (float) Math.max(1, s.width); }
    public float getYScale() { return d.height / (float) Math.max(1, s.height); }
}
