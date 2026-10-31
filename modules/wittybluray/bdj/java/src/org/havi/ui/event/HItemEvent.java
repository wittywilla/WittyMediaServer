package org.havi.ui.event;
public class HItemEvent extends java.awt.AWTEvent {
    public static final int ITEM_FIRST = 2006, ITEM_START_CHANGE = 2006, ITEM_TOGGLE_SELECTED = 2007, ITEM_SELECTED = 2008, ITEM_CLEARED = 2009, ITEM_SELECTION_CLEARED = 2010,
        ITEM_SET_CURRENT = 2011, ITEM_SET_PREVIOUS = 2012, ITEM_SET_NEXT = 2013, SCROLL_MORE = 2014, SCROLL_LESS = 2015, SCROLL_PAGE_MORE = 2016, SCROLL_PAGE_LESS = 2017, ITEM_END_CHANGE = 2018, ITEM_LAST = 2018;
    private final Object item;
    public HItemEvent(org.havi.ui.HSelectionInputPreferred s, int id, Object i) { super(s, id); item = i; }
    public Object getItem() { return item; }
}
