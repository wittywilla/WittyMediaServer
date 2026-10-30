package javax.media;
public final class Manager {
    public static final String UNKNOWN_CONTENT_NAME = "unknown";
    private Manager() {}
    public static Player createPlayer(MediaLocator l) throws java.io.IOException, NoPlayerException { return org.bluray.media.BDPlayer.create(l.toExternalForm()); }
    public static Player createPlayer(java.net.URL u) throws java.io.IOException, NoPlayerException { return org.bluray.media.BDPlayer.create(String.valueOf(u)); }
    public static Player createPlayer(witty.shim.URL u) throws java.io.IOException, NoPlayerException { return org.bluray.media.BDPlayer.create(String.valueOf(u)); }
    public static Player createPlayer(javax.media.protocol.DataSource s) throws java.io.IOException, NoPlayerException { return org.bluray.media.BDPlayer.create(String.valueOf(s.getLocator())); }
    public static TimeBase getSystemTimeBase() { return org.bluray.media.BDPlayer.SYSTEM; }
    public static String getVersion() { return "1.0"; }
    public static void setHint(int h, Object v) {}
    public static Object getHint(int h) { return null; }
}
