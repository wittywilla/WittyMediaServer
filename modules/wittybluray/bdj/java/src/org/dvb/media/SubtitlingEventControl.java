package org.dvb.media;
public interface SubtitlingEventControl extends javax.media.Control {
    void addSubtitleListener(SubtitleListener l);
    void removeSubtitleListener(SubtitleListener l);
}
