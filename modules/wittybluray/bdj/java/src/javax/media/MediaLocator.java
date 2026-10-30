package javax.media;
public class MediaLocator {
    private final String s;
    public MediaLocator(String v) { s = v; }
    public MediaLocator(java.net.URL u) { s = String.valueOf(u); }
    public MediaLocator(witty.shim.URL u) { s = String.valueOf(u); }
    public String getProtocol() { int i = s.indexOf(':'); return i < 0 ? "" : s.substring(0, i); }
    public String getRemainder() { int i = s.indexOf(':'); return i < 0 ? s : s.substring(i + 1); }
    public String toExternalForm() { return s; }
    public String toString() { return s; }
}
