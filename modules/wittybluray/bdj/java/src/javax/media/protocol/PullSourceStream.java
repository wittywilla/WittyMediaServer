package javax.media.protocol;
public interface PullSourceStream extends SourceStream {
    boolean willReadBlock();
    int read(byte[] buffer, int offset, int length) throws java.io.IOException;
}
