package org.bluray.media;
public interface PlaybackControl extends javax.media.Control {
    void addPlaybackControlListener(PlaybackListener l);
    void removePlaybackControlListener(PlaybackListener l);
    void skipToMark(int mark) throws IllegalArgumentException;
    boolean skipToNextMark(int type) throws IllegalArgumentException;
    boolean skipToPreviousMark(int type) throws IllegalArgumentException;
    void skipToPlayItem(int item) throws IllegalArgumentException;
}
