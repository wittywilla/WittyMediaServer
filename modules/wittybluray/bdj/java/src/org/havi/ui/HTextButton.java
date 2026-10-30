package org.havi.ui;
public class HTextButton extends HButtonBase {
    public HTextButton() { super(new HTextLook()); }
    public HTextButton(String t) { this(); setTextContent(t, ALL_STATES); }
    public HTextButton(String t, int x, int y, int w, int h) { this(t); setBounds(x, y, w, h); }
    public HTextButton(String t, java.awt.Font f, java.awt.Color fg, java.awt.Color bg, HTextLayoutManager m) { this(t); setFont(f); setForeground(fg); setBackground(bg); }
    public HTextButton(String t, int x, int y, int w, int h, java.awt.Font f, java.awt.Color fg, java.awt.Color bg, HTextLayoutManager m) { this(t, x, y, w, h); setFont(f); setForeground(fg); setBackground(bg); }
}
