package org.bluray.media;
public interface SubtitlingControl extends StreamControl, org.davic.media.SubtitlingLanguageControl {
    boolean setSubtitling(boolean on);
    boolean isSubtitlingOn();
    boolean isSubtitlingAvailable();
}
