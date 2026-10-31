package org.havi.ui.event;
public class HAdjustmentEvent extends java.awt.AWTEvent {
    public static final int ADJUST_FIRST = 2000, ADJUST_LESS = 2000, ADJUST_MORE = 2001, ADJUST_PAGE_LESS = 2002, ADJUST_PAGE_MORE = 2003, ADJUST_START_CHANGE = 2004, ADJUST_END_CHANGE = 2005, ADJUST_LAST = 2005;
    public HAdjustmentEvent(org.havi.ui.HAdjustmentInputPreferred s, int id) { super(s, id); }
}
