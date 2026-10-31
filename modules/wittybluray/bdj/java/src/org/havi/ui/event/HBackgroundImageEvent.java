package org.havi.ui.event;
public class HBackgroundImageEvent extends java.util.EventObject {
    public static final int BACKGROUNDIMAGE_FIRST = 1, BACKGROUNDIMAGE_LOADED = 1, BACKGROUNDIMAGE_FILE_NOT_FOUND = 2, BACKGROUNDIMAGE_IOERROR = 3, BACKGROUNDIMAGE_INVALID = 4, BACKGROUNDIMAGE_LAST = 4;
    private final int id;
    public HBackgroundImageEvent(Object s, int id) { super(s); this.id = id; }
    public int getID() { return id; }
}
