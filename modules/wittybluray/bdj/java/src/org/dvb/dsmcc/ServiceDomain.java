package org.dvb.dsmcc;

/** The root folder of the disc is the one service domain there is. */
public class ServiceDomain {
    private volatile boolean attached;
    private volatile org.davic.net.Locator where;

    public ServiceDomain() {}
    private static boolean ours(org.davic.net.Locator l) {
        if (l == null) return false;
        String s = l.toExternalForm();
        return s != null && (s.startsWith("bd:") || s.startsWith("file:"));
    }
    public void attach(org.davic.net.Locator l) throws DSMCCException, java.io.InterruptedIOException, MPEGDeliveryException {
        if (!ours(l)) throw new MPEGDeliveryException("no broadcast is available");
        where = l;
        attached = true;
    }
    public void attach(org.davic.net.Locator l, int carouselId) throws DSMCCException, java.io.InterruptedIOException, MPEGDeliveryException { attach(l); }
    public void attach(byte[] nsap) throws DSMCCException, java.io.InterruptedIOException, MPEGDeliveryException { throw new MPEGDeliveryException("no broadcast is available"); }
    public void attach(byte[] nsap, int carouselId) throws DSMCCException, java.io.InterruptedIOException, MPEGDeliveryException { throw new MPEGDeliveryException("no broadcast is available"); }
    public void detach() throws NotLoadedException {
        if (!attached) throw new NotLoadedException("not attached");
        attached = false;
    }
    public boolean isAttached() { return attached; }
    public boolean isNetworkConnectionAvailable() { return false; }
    public DSMCCObject getMountPoint() { return attached ? new DSMCCObject(witty.shim.System.getProperty("bluray.vfs.root")) : null; }
    public org.davic.net.Locator getLocator() { return where; }
    public byte[] getNSAPAddress() { return new byte[20]; }
    public witty.shim.URL getURL() { DSMCCObject m = getMountPoint(); return m == null ? null : m.getURL(); }
}
