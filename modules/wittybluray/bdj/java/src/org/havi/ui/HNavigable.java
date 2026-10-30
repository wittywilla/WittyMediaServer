package org.havi.ui;
public interface HNavigable extends HNavigationInputPreferred {
    void setMove(int code, HNavigable to);
    HNavigable getMove(int code);
    void setFocusTraversal(HNavigable up, HNavigable down, HNavigable left, HNavigable right);
    boolean isSelected();
    void setGainFocusSound(HSound s);
    void setLoseFocusSound(HSound s);
    HSound getGainFocusSound();
    HSound getLoseFocusSound();
    void addHFocusListener(org.havi.ui.event.HFocusListener l);
    void removeHFocusListener(org.havi.ui.event.HFocusListener l);
}
