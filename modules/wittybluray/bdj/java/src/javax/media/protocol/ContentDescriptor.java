package javax.media.protocol;
public class ContentDescriptor extends javax.media.Format {
    public static final String RAW = "raw", RAW_RTP = "raw.rtp", MIXED = "application.mixed-data", CONTENT_UNKNOWN = "UnknownContent";
    public ContentDescriptor(String cdName) { super(cdName); }
    public String getContentType() { return encoding; }
    public String toString() { return "ContentDescriptor: " + encoding; }
    /** Turns a MIME type into a name usable in a class name: lower case, slash to dot, anything else odd to underscore. */
    public static final String mimeTypeToPackageName(String mime) {
        if (mime == null) return null;
        StringBuilder b = new StringBuilder();
        for (char c : mime.toLowerCase().toCharArray()) b.append(c == '/' ? '.' : (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '.' ? c : '_');
        return b.toString();
    }
}
