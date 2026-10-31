package org.havi.ui.event;
public class HFocusEvent extends java.awt.event.FocusEvent {
    public static final int FOCUS_TRANSFER = 1006;
    public static final int NO_TRANSFER_ID = -1;
    private final int transfer;
    public HFocusEvent(java.awt.Component s, int id) { this(s, id, NO_TRANSFER_ID); }
    public HFocusEvent(java.awt.Component s, int id, int t) { super(s, id); transfer = t; }
    public int getTransferId() { return transfer; }
}
