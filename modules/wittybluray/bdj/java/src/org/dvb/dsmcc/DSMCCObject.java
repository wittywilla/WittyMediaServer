package org.dvb.dsmcc;

/** The files of the disc stand in for the objects of a broadcast carousel, so loading is immediate. */
public class DSMCCObject extends witty.shim.File {
    private volatile boolean loaded;

    public DSMCCObject(String path) { super(abs(path)); }
    public DSMCCObject(String path, String name) { super(abs(path), name); }
    public DSMCCObject(DSMCCObject dir, String name) { super(dir, name); }

    static String abs(String p) {
        String root = witty.shim.System.getProperty("bluray.vfs.root");
        if (p == null || root == null || root.length() == 0 || p.startsWith(root)) return p;
        int i = 0;
        while (i < p.length() && (p.charAt(i) == '/' || p.charAt(i) == '\\')) i++;
        return root + separator + p.substring(i);
    }

    public void asynchronousLoad(final AsynchronousLoadingEventListener l) throws InvalidPathNameException {
        if (l == null) throw new NullPointerException();
        final AsynchronousLoadingEvent e;
        if (exists()) {
            loaded = true;
            e = new SuccessEvent(this);
        } else {
            e = new InvalidPathnameEvent(this);
        }
        Thread t = new Thread(new Runnable() {
            public void run() { try { l.receiveEvent(e); } catch (Throwable x) {} }
        }, "dsmcc-load");
        t.setDaemon(true);
        t.start();
    }
    public void synchronousLoad() throws InvalidPathNameException, InsufficientResourcesException, InvalidFormatException, MPEGDeliveryException, ServerDeliveryException, NotEntitledException, java.io.InterruptedIOException {
        if (!exists()) throw new InvalidPathNameException(getName());
        loaded = true;
    }
    public void abort() throws NothingToAbortException { throw new NothingToAbortException("loading is immediate"); }
    public void unload() throws NotLoadedException { loaded = false; }
    public boolean isLoaded() { return loaded; }
    public boolean isObject() { return exists(); }
    public boolean isStream() { return false; }
    public boolean isStreamEvent() { return false; }
    public boolean isEntitled() { return true; }
    public void prefetch(String path, byte priority) {}
    public void addObjectChangeEventListener(ObjectChangeEventListener l) throws InsufficientResourcesException {}
    public void removeObjectChangeEventListener(ObjectChangeEventListener l) {}
    public java.security.cert.Certificate[][] getSigners() { return new java.security.cert.Certificate[0][]; }
    public witty.shim.URL getURL() { try { return toURL(); } catch (java.net.MalformedURLException e) { return null; } }
}
