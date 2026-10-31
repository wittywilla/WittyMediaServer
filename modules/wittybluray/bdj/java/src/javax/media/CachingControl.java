package javax.media;
public interface CachingControl extends Control {
    long LENGTH_UNKNOWN = Long.MAX_VALUE;
    boolean isDownloading();
    long getContentLength();
    long getContentProgress();
    java.awt.Component getProgressBarComponent();
    java.awt.Component getStatusComponent();
}
