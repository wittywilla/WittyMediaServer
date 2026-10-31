package org.bluray.media;
public interface PiPControl extends javax.media.Control {
    void setDisplay(boolean on);
    boolean getIsDisplay();
    void setFullScreen(boolean on);
    boolean getFullScreen();
    boolean isSynchronized();
    void addPiPStatusListener(PiPStatusListener l);
    void removePiPStatusListener(PiPStatusListener l);
}
