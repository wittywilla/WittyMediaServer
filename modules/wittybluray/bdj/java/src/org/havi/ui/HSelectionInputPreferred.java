package org.havi.ui;
public interface HSelectionInputPreferred {
    boolean getSelectionMode();
    void setSelectionMode(boolean on);
    void processHItemEvent(org.havi.ui.event.HItemEvent e);
}
