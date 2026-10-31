package org.havi.ui;

import java.awt.Image;
import org.havi.ui.event.HBackgroundImageEvent;
import org.havi.ui.event.HBackgroundImageListener;

/** A picture for the background; decoded by the same sandboxed path as any other picture of a disc. */
public class HBackgroundImage {
    private final String path;
    private final witty.shim.URL url;
    private final byte[] bytes;
    private volatile Image pic;
    public HBackgroundImage(String f) { path = f; url = null; bytes = null; }
    public HBackgroundImage(witty.shim.URL u) { path = null; url = u; bytes = null; }
    public HBackgroundImage(byte[] b) { path = null; url = null; bytes = b; }
    public void load(final HBackgroundImageListener l) {
        Thread t = new Thread(new Runnable() {
            public void run() {
                final boolean ok = picture() != null;
                final int id = ok ? HBackgroundImageEvent.BACKGROUNDIMAGE_LOADED : path != null && !new witty.shim.File(path).exists() ? HBackgroundImageEvent.BACKGROUNDIMAGE_FILE_NOT_FOUND : HBackgroundImageEvent.BACKGROUNDIMAGE_INVALID;
                if (l == null) return;
                HComponent.later(new Runnable() {
                    public void run() {
                        HBackgroundImageEvent e = new HBackgroundImageEvent(HBackgroundImage.this, id);
                        if (ok) l.imageLoaded(e);
                        else l.imageLoadFailed(e);
                    }
                });
            }
        }, "havi-background");
        t.setDaemon(true);
        t.start();
    }
    final synchronized Image picture() {
        if (pic != null) return pic;
        witty.shim.Toolkit k = witty.shim.Toolkit.getDefaultToolkit();
        Image i = null;
        if (path != null) i = k.createImage(path);
        else if (url != null) i = k.createImage(url);
        else if (bytes != null) i = k.createImage(bytes);
        // A picture that could not be read comes back as one empty pixel.
        if (i != null && i.getWidth(null) > 1 && i.getHeight(null) > 1) pic = i;
        return pic;
    }
    public int getHeight() { Image i = pic; return i == null ? -1 : i.getHeight(null); }
    public int getWidth() { Image i = pic; return i == null ? -1 : i.getWidth(null); }
    public void flush() { pic = null; }
}
