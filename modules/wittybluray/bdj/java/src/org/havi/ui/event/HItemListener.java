package org.havi.ui.event;
public interface HItemListener extends java.util.EventListener {
    void selectionChanged(HItemEvent e);
    void currentItemChanged(HItemEvent e);
}
