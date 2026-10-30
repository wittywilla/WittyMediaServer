package org.havi.ui;
public interface HActionable extends HNavigable, HActionInputPreferred {
    void addHActionListener(org.havi.ui.event.HActionListener l);
    void removeHActionListener(org.havi.ui.event.HActionListener l);
    void setActionCommand(String c);
    void setActionSound(HSound s);
    HSound getActionSound();
}
