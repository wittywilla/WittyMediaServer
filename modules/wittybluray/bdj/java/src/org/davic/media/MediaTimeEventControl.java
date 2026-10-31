package org.davic.media;
public interface MediaTimeEventControl extends javax.media.Control {
    void notifyWhen(MediaTimeEventListener l, javax.media.Time mediaTime);
    void removeMediaTimeEventListener(MediaTimeEventListener l);
}
