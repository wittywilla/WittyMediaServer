package org.dvb.event;
public class UserEvent extends java.util.EventObject {
    public static final int UEF_KEY_EVENT = 1;
    private final int family, type, code, mods;
    private final long when;
    public UserEvent(Object s, int f, int t, int c, int m, long w) { super(s); family = f; type = t; code = c; mods = m; when = w; }
    public UserEvent(Object s, int f, char ch, long w) { this(s, f, java.awt.event.KeyEvent.KEY_TYPED, ch, 0, w); }
    public int getFamily() { return family; }
    public int getType() { return type; }
    public int getCode() { return code; }
    public char getKeyChar() { return (char) code; }
    public int getModifiers() { return mods; }
    public long getWhen() { return when; }
    public boolean isShiftDown() { return false; }
    public boolean isControlDown() { return false; }
    public boolean isAltDown() { return false; }
    public boolean isMetaDown() { return false; }
}
