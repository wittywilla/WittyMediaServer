package org.dvb.media;
public interface BackgroundVideoPresentationControl extends VideoPresentationControl {
    boolean setVideoTransformation(VideoTransformation t);
    VideoTransformation getVideoTransformation();
    VideoTransformation getClosestMatch(VideoTransformation t);
}
