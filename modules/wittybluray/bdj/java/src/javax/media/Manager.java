package javax.media;
import java.util.HashMap;
import java.util.Map;
import java.util.Vector;

public final class Manager {
    public static final String UNKNOWN_CONTENT_NAME = "unknown";
    public static final int CACHING = 1, LIGHTWEIGHT_RENDERER = 2, PLUGIN_PLAYER = 3, MAX_SECURITY = 4;
    private static final Map<Integer, Object> hints = new HashMap<Integer, Object>();
    private Manager() {}
    public static Player createPlayer(MediaLocator l) throws java.io.IOException, NoPlayerException {
        if (l == null) throw new NoPlayerException("no locator");
        return org.bluray.media.BDPlayer.create(l.toExternalForm());
    }
    public static Player createPlayer(witty.shim.URL u) throws java.io.IOException, NoPlayerException {
        if (u == null) throw new NoPlayerException("no URL");
        return org.bluray.media.BDPlayer.create(String.valueOf(u));
    }
    public static Player createPlayer(javax.media.protocol.DataSource s) throws java.io.IOException, NoPlayerException {
        if (s == null || s.getLocator() == null) throw new NoPlayerException("no locator");
        return org.bluray.media.BDPlayer.create(String.valueOf(s.getLocator()));
    }
    public static Player createRealizedPlayer(MediaLocator l) throws java.io.IOException, NoPlayerException, CannotRealizeException { return realized(createPlayer(l)); }
    public static Player createRealizedPlayer(witty.shim.URL u) throws java.io.IOException, NoPlayerException, CannotRealizeException { return realized(createPlayer(u)); }
    public static Player createRealizedPlayer(javax.media.protocol.DataSource s) throws java.io.IOException, NoPlayerException, CannotRealizeException { return realized(createPlayer(s)); }
    /** Realizing is asynchronous in the player; wait for it, but never for long. */
    private static Player realized(Player p) throws CannotRealizeException {
        p.realize();
        long end = System.currentTimeMillis() + 5000;
        while (p.getState() < Controller.Realized) {
            if (System.currentTimeMillis() > end) {
                p.close();
                throw new CannotRealizeException("realizing took too long");
            }
            try {
                Thread.sleep(10);
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                p.close();
                throw new CannotRealizeException("interrupted");
            }
        }
        return p;
    }
    /** Only files and resources of the disc can be read as data; bd: locators are played, not read. */
    public static javax.media.protocol.DataSource createDataSource(MediaLocator l) throws java.io.IOException, NoDataSourceException {
        if (l == null || l.getProtocol().equals("bd") || l.getProtocol().length() == 0) throw new NoDataSourceException("no data source for " + l);
        return createDataSource(l.getURL());
    }
    public static javax.media.protocol.DataSource createDataSource(witty.shim.URL u) throws java.io.IOException, NoDataSourceException {
        if (u == null) throw new NoDataSourceException("no URL");
        return new javax.media.protocol.URLDataSource(u);
    }
    public static TimeBase getSystemTimeBase() { return org.bluray.media.BDPlayer.SYSTEM; }
    public static String getVersion() { return "1.0"; }
    public static Vector<String> getDataSourceList(String protocolName) { return new Vector<String>(); }
    public static Vector<String> getHandlerClassList(String contentName) { return new Vector<String>(); }
    public static void setHint(int h, Object v) {
        synchronized (hints) {
            if (v == null) hints.remove(Integer.valueOf(h));
            else if (hints.size() < 16) hints.put(Integer.valueOf(h), v);
        }
    }
    public static Object getHint(int h) {
        synchronized (hints) {
            return hints.get(Integer.valueOf(h));
        }
    }
}
