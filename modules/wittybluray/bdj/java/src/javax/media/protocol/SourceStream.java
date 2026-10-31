package javax.media.protocol;
public interface SourceStream extends javax.media.Controls {
    long LENGTH_UNKNOWN = -1L;
    ContentDescriptor getContentDescriptor();
    long getContentLength();
    boolean endOfStream();
}
