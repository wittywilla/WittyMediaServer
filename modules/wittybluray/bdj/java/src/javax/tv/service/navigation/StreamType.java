package javax.tv.service.navigation;
public class StreamType {
    public static final StreamType VIDEO = new StreamType("VIDEO");
    public static final StreamType AUDIO = new StreamType("AUDIO");
    public static final StreamType DATA = new StreamType("DATA");
    public static final StreamType SUBTITLES = new StreamType("SUBTITLES");
    public static final StreamType SECTIONS = new StreamType("SECTIONS");
    public static final StreamType UNKNOWN = new StreamType("UNKNOWN");
    private final String n;
    protected StreamType(String s) { n = s; }
    public String toString() { return n; }
}
