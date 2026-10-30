package org.bluray.media;
public interface UOMaskTableControl extends javax.media.Control {
    boolean[] getMaskedUOTable();
    void addUOMaskTableChangeListener(Object l);
    void removeUOMaskTableChangeListener(Object l);
}
