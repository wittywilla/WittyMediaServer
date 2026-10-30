package org.havi.ui.event;
public class HBackgroundImageEvent extends java.util.EventObject {
    public static final int BACKGROUNDIMAGE_FIRST = 1, BACKGROUND_LOADED = 1, BACKGROUND_ERROR = 2, BACKGROUNDIMAGE_LAST = 3;
    private final int id;
    public HBackgroundImageEvent(Object s, int id) { super(s); this.id = id; }
    public int getID() { return id; }
}
