package org.havi.ui;
public interface HItemValue extends HNavigable, HSelectionInputPreferred {
    void addItemListener(org.havi.ui.event.HItemListener l);
    void removeItemListener(org.havi.ui.event.HItemListener l);
    void setSelectionSound(HSound s);
    HSound getSelectionSound();
}
