package org.havi.ui.event;
public interface HBackgroundImageListener extends java.util.EventListener {
    void imageLoaded(HBackgroundImageEvent e);
    void imageLoadFailed(HBackgroundImageEvent e);
}
