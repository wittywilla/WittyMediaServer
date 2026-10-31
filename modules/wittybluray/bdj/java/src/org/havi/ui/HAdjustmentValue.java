package org.havi.ui;
public interface HAdjustmentValue extends HNavigable, HAdjustmentInputPreferred {
    void setUnitIncrement(int n);
    int getUnitIncrement();
    void setBlockIncrement(int n);
    int getBlockIncrement();
    void addAdjustmentListener(org.havi.ui.event.HAdjustmentListener l);
    void removeAdjustmentListener(org.havi.ui.event.HAdjustmentListener l);
    void setAdjustmentSound(HSound s);
    HSound getAdjustmentSound();
}
