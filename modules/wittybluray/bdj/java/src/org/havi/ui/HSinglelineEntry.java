package org.havi.ui;

import java.awt.Color;
import java.awt.Font;
import java.awt.event.KeyEvent;
import java.util.List;
import org.havi.ui.event.HKeyEvent;
import org.havi.ui.event.HKeyListener;
import org.havi.ui.event.HTextEvent;
import org.havi.ui.event.HTextListener;

public class HSinglelineEntry extends HVisible implements HTextValue {
    private static HSinglelineEntryLook look = new HSinglelineEntryLook();
    public static void setDefaultLook(HSinglelineEntryLook l) { look = l; }
    public static HSinglelineEntryLook getDefaultLook() { return look; }
    private final List<HTextListener> texts = new java.util.concurrent.CopyOnWriteArrayList<HTextListener>();
    private final List<HKeyListener> keys = new java.util.concurrent.CopyOnWriteArrayList<HKeyListener>();
    private String text = "";
    private int caret, max = 16, type = INPUT_ANY;
    private char echo;
    private char[] valid;
    private boolean editing;
    public HSinglelineEntry() { super(look); }
    HSinglelineEntry(HSinglelineEntryLook l) { super(l); }
    public HSinglelineEntry(int maxChars) { this(); setMaxChars(maxChars); }
    public HSinglelineEntry(int x, int y, int w, int h, int maxChars) { this(maxChars); setBounds(x, y, w, h); }
    public HSinglelineEntry(String t, int x, int y, int w, int h, int maxChars, Font f, Color c) { this(x, y, w, h, maxChars); start(t, f, c); }
    public HSinglelineEntry(String t, int maxChars, Font f, Color c) { this(maxChars); start(t, f, c); }
    final void start(String t, Font f, Color c) { setFont(f); setForeground(c); setTextContent(t, ALL_STATES); }
    /** The text, for looks: one string whatever the state. */
    final String getText() { return text; }
    public void setTextContent(String t, int mask) {
        text = t == null ? "" : t.length() > max ? t.substring(0, max) : t;
        caret = Math.min(caret, text.length());
        super.setTextContent(text, ALL_STATES);
    }
    public int getCaretCharPosition() { return caret; }
    public int setCaretCharPosition(int p) {
        caret = Math.max(0, Math.min(text.length(), p));
        changed(CARET_POSITION_CHANGE, null);
        return caret;
    }
    public void caretNextCharacter() { setCaretCharPosition(caret + 1); }
    public void caretPreviousCharacter() { setCaretCharPosition(caret - 1); }
    public void setType(int t) { type = t; }
    public int getType() { return type; }
    public void setValidInput(char[] c) { valid = c == null ? null : (char[]) c.clone(); }
    public char[] getValidInput() { return valid == null ? null : (char[]) valid.clone(); }
    public boolean echoCharIsSet() { return echo != 0; }
    public char getEchoChar() { return echo; }
    public void setEchoChar(char c) { echo = c; changed(ECHO_CHAR_CHANGE, null); }
    public int getMaxChars() { return max; }
    public void setMaxChars(int m) { if (m > 0) { max = m; setTextContent(text, ALL_STATES); } }
    public boolean getEditMode() { return editing; }
    public void setEditMode(boolean on) { editing = on; setStateBit(ACTIONED_STATE_BIT, on); changed(EDIT_MODE_CHANGE, null); }
    public void addHKeyListener(HKeyListener l) { if (l != null) keys.add(l); }
    public void removeHKeyListener(HKeyListener l) { keys.remove(l); }
    public void addHTextListener(HTextListener l) { if (l != null) texts.add(l); }
    public void removeHTextListener(HTextListener l) { texts.remove(l); }
    public void processHFocusEvent(org.havi.ui.event.HFocusEvent e) {
        if (e.getID() == org.havi.ui.event.HFocusEvent.FOCUS_LOST && editing) setEditMode(false);
        super.processHFocusEvent(e);
    }
    public void processHTextEvent(HTextEvent e) {
        if ((getInteractionState() & DISABLED_STATE_BIT) != 0) return;
        switch (e.getID()) {
            case HTextEvent.TEXT_START_CHANGE: setEditMode(true); fireText(e); return;
            case HTextEvent.TEXT_END_CHANGE: setEditMode(false); fireText(e); return;
            case HTextEvent.TEXT_CHANGE: fireText(e); return;
            case HTextEvent.CARET_NEXT_CHAR: caretNextCharacter(); break;
            case HTextEvent.CARET_PREV_CHAR: caretPreviousCharacter(); break;
            default: break;
        }
        fireCaret(e);
    }
    final void fireText(HTextEvent e) { for (HTextListener l : texts) l.textChanged(e); }
    final void fireCaret(HTextEvent e) { for (HTextListener l : texts) l.caretMoved(e); }
    public void processHKeyEvent(HKeyEvent e) {
        if (e.getID() == KeyEvent.KEY_PRESSED && editing) {
            int code = e.getKeyCode();
            char c = e.getKeyChar();
            if (code == KeyEvent.VK_BACK_SPACE) { if (caret > 0) put(text.substring(0, caret - 1) + text.substring(caret), caret - 1); }
            else if (code == KeyEvent.VK_DELETE) { if (caret < text.length()) put(text.substring(0, caret) + text.substring(caret + 1), caret); }
            else if (accepts(c)) put(text.substring(0, caret) + c + text.substring(caret), caret + 1);
        }
        for (HKeyListener l : keys) {
            if (e.getID() == KeyEvent.KEY_PRESSED) l.keyPressed(e);
            else if (e.getID() == KeyEvent.KEY_RELEASED) l.keyReleased(e);
            else l.keyTyped(e);
        }
    }
    private void put(String t, int at) {
        setTextContent(t, ALL_STATES);
        setCaretCharPosition(at);
        fireText(new HTextEvent(this, HTextEvent.TEXT_CHANGE));
    }
    private boolean accepts(char c) {
        if (c == KeyEvent.CHAR_UNDEFINED || Character.isISOControl(c) || text.length() >= max) return false;
        if ((type & INPUT_ANY) != 0) return true;
        if ((type & INPUT_NUMERIC) != 0 && Character.isDigit(c)) return true;
        if ((type & INPUT_ALPHA) != 0 && Character.isLetter(c)) return true;
        if ((type & INPUT_CUSTOMIZED) != 0 && valid != null) for (int i = 0; i < valid.length; i++) if (valid[i] == c) return true;
        return false;
    }
}
