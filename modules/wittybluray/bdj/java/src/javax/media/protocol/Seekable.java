package javax.media.protocol;
public interface Seekable {
    long seek(long where);
    long tell();
    boolean isRandomAccess();
}
