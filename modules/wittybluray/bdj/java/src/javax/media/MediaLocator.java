package javax.media;
public class MediaLocator {
    private final String s;
    public MediaLocator(String v) { s = v == null ? "" : v; }
    public MediaLocator(witty.shim.URL u) { s = String.valueOf(u); }
    public witty.shim.URL getURL() throws java.net.MalformedURLException { return new witty.shim.URL(s); }
    public String getProtocol() { int i = s.indexOf(':'); return i < 0 ? "" : s.substring(0, i); }
    public String getRemainder() { int i = s.indexOf(':'); return i < 0 ? s : s.substring(i + 1); }
    public String toExternalForm() { return s; }
    public String toString() { return s; }
}
