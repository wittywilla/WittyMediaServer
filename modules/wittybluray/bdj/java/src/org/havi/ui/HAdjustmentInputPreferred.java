package org.havi.ui;
public interface HAdjustmentInputPreferred {
    boolean getAdjustMode();
    void setAdjustMode(boolean on);
    void processHAdjustmentEvent(org.havi.ui.event.HAdjustmentEvent e);
}
