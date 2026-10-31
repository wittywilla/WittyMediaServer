package org.havi.ui;
public interface HTextValue extends HNavigable, HKeyboardInputPreferred {
    void addHKeyListener(org.havi.ui.event.HKeyListener l);
    void removeHKeyListener(org.havi.ui.event.HKeyListener l);
    void addHTextListener(org.havi.ui.event.HTextListener l);
    void removeHTextListener(org.havi.ui.event.HTextListener l);
}
