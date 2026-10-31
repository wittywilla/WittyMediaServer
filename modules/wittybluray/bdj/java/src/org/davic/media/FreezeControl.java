package org.davic.media;
public interface FreezeControl extends javax.media.Control {
    void freeze() throws MediaFreezeException;
    void resume() throws MediaFreezeException;
}
