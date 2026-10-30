package org.bluray.media;
public interface PlaybackListener extends java.util.EventListener {
    void markReached(PlaybackMarkEvent e);
    void playItemReached(PlaybackPlayItemEvent e);
}
