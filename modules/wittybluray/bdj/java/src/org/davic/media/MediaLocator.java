package org.davic.media;
public class MediaLocator extends javax.media.MediaLocator {
    public MediaLocator(String s) { super(s); }
    public MediaLocator(org.davic.net.Locator l) { super(l.toExternalForm()); }
}
