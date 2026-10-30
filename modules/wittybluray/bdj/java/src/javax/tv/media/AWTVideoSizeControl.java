package javax.tv.media;
public interface AWTVideoSizeControl extends javax.media.Control {
    AWTVideoSize getSize();
    AWTVideoSize getDefaultSize();
    java.awt.Dimension getSourceVideoSize();
    boolean setSize(AWTVideoSize s);
    AWTVideoSize checkSize(AWTVideoSize s);
}
