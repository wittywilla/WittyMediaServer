package javax.media;
public interface ExtendedCachingControl extends CachingControl {
    void addDownloadProgressListener(DownloadProgressListener l, int numIntervals);
    void removeDownloadProgressListener(DownloadProgressListener l);
    long getStartOffset();
    long getEndOffset();
    void pauseDownload();
    void resumeDownload();
}
