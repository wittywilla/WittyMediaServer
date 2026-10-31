package org.havi.ui.event;
public interface HTextListener extends java.util.EventListener {
    void textChanged(HTextEvent e);
    void caretMoved(HTextEvent e);
}
