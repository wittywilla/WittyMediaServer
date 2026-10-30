package org.havi.ui.event;
public class HFocusEvent extends java.awt.event.FocusEvent {
    public static final int FOCUS_TRANSFER = 1006;
    public static final int NO_TRANSFER_ID = -1;
    public HFocusEvent(java.awt.Component s, int id) { super(s, id); }
    public HFocusEvent(java.awt.Component s, int id, int transfer) { super(s, id); }
    public int getTransferId() { return NO_TRANSFER_ID; }
}
