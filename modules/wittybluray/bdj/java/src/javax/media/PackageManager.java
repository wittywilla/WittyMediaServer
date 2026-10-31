package javax.media;
import java.util.Vector;

/** The package prefix lists a player would search for handlers. This player has one built-in handler, so the lists are only remembered, never used to load classes. */
public class PackageManager {
    private static Vector<String> protocols = defaults(), contents = defaults();
    private static Vector<String> defaults() {
        Vector<String> v = new Vector<String>();
        v.addElement("javax");
        return v;
    }
    public static synchronized Vector<String> getProtocolPrefixList() { return new Vector<String>(protocols); }
    public static synchronized void setProtocolPrefixList(Vector<String> l) { if (l != null) protocols = new Vector<String>(l); }
    public static void commitProtocolPrefixList() {}
    public static synchronized Vector<String> getContentPrefixList() { return new Vector<String>(contents); }
    public static synchronized void setContentPrefixList(Vector<String> l) { if (l != null) contents = new Vector<String>(l); }
    public static void commitContentPrefixList() {}
}
