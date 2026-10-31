package org.bluray.media;
public interface UOMaskTableControl extends javax.media.Control {
    boolean[] getMaskedUOTable();
    void addUOMaskTableEventListener(UOMaskTableListener l);
    void removeUOMaskTableEventListener(UOMaskTableListener l);
}
