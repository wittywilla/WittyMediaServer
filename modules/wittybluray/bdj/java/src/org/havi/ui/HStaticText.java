package org.havi.ui;

import java.awt.Color;
import java.awt.Font;

public class HStaticText extends HVisible implements HNoInputPreferred {
    private static HTextLook look = new HTextLook();
    public static void setDefaultLook(HTextLook l) { look = l; }
    public static HTextLook getDefaultLook() { return look; }
    public HStaticText() { super(look); }
    public HStaticText(String t) { this(); setTextContent(t, ALL_STATES); }
    public HStaticText(String t, int x, int y, int w, int h) { this(t); setBounds(x, y, w, h); }
    public HStaticText(String t, Font f, Color fg, Color bg, HTextLayoutManager m) { this(t); paintWith(f, fg, bg, m); }
    public HStaticText(String t, int x, int y, int w, int h, Font f, Color fg, Color bg, HTextLayoutManager m) { this(t, x, y, w, h); paintWith(f, fg, bg, m); }
    final void paintWith(Font f, Color fg, Color bg, HTextLayoutManager m) { setFont(f); setForeground(fg); setBackground(bg); if (m != null) setTextLayoutManager(m); }
}
