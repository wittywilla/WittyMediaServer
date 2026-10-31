package org.havi.ui;
public interface HSwitchable extends HActionable {
    boolean getSwitchableState();
    void setSwitchableState(boolean on);
    void setUnsetActionSound(HSound s);
    HSound getUnsetActionSound();
}
