package org.havi.ui;
public class HStaticText extends HVisible {
    public HStaticText() { super(new HTextLook()); }
    public HStaticText(String t) { this(); setTextContent(t, ALL_STATES); }
    public HStaticText(String t, int x, int y, int w, int h) { this(t); setBounds(x, y, w, h); }
    public HStaticText(String t, java.awt.Font f, java.awt.Color fg, java.awt.Color bg, HTextLayoutManager m) { this(t); setFont(f); setForeground(fg); setBackground(bg); }
    public HStaticText(String t, int x, int y, int w, int h, java.awt.Font f, java.awt.Color fg, java.awt.Color bg, HTextLayoutManager m) { this(t, x, y, w, h); setFont(f); setForeground(fg); setBackground(bg); }
}
