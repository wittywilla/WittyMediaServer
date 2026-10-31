package org.havi.ui;

import java.awt.Color;
import java.awt.Font;
import org.havi.ui.event.HTextEvent;

public class HMultilineEntry extends HSinglelineEntry {
    private static HMultilineEntryLook look = new HMultilineEntryLook();
    public static void setDefaultLook(HMultilineEntryLook l) { look = l; }
    public static HMultilineEntryLook getDefaultLook() { return look; }
    public HMultilineEntry() { super(look); }
    public HMultilineEntry(int maxChars) { this(); setMaxChars(maxChars); }
    public HMultilineEntry(int x, int y, int w, int h, int maxChars) { this(maxChars); setBounds(x, y, w, h); }
    public HMultilineEntry(String t, int x, int y, int w, int h, int maxChars, Font f, Color c) { this(x, y, w, h, maxChars); start(t, f, c); }
    public HMultilineEntry(String t, int maxChars, Font f, Color c) { this(maxChars); start(t, f, c); }
    public void caretNextLine() { setCaretCharPosition(caretOnLine(1)); }
    public void caretPreviousLine() { setCaretCharPosition(caretOnLine(-1)); }
    public void caretNextPage() { for (int i = 0; i < 8; i++) caretNextLine(); }
    public void caretPreviousPage() { for (int i = 0; i < 8; i++) caretPreviousLine(); }
    /** Where the caret lands a line up or down, as far along the line as it is now. */
    final int caretOnLine(int by) {
        String t = getText();
        int caret = getCaretCharPosition();
        int start = t.lastIndexOf('\n', caret - 1) + 1, along = caret - start;
        if (by > 0) {
            int end = t.indexOf('\n', caret);
            if (end < 0) return caret;
            int next = t.indexOf('\n', end + 1);
            return Math.min(end + 1 + along, next < 0 ? t.length() : next);
        }
        if (start == 0) return caret;
        int up = t.lastIndexOf('\n', start - 2) + 1;
        return Math.min(up + along, start - 1);
    }
    public void processHTextEvent(HTextEvent e) {
        switch (e.getID()) {
            case HTextEvent.CARET_NEXT_LINE: caretNextLine(); break;
            case HTextEvent.CARET_PREV_LINE: caretPreviousLine(); break;
            case HTextEvent.CARET_NEXT_PAGE: caretNextPage(); break;
            case HTextEvent.CARET_PREV_PAGE: caretPreviousPage(); break;
            default: super.processHTextEvent(e); return;
        }
        fireCaret(e);
    }
}
