package org.davic.media;
public interface MediaTimePositionControl extends javax.media.Control {
    javax.media.Time setMediaTimePosition(javax.media.Time t) throws MediaTimeOutOfBoundsException;
    javax.media.Time getMediaTimePosition();
}
