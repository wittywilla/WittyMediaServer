package org.havi.ui;

import java.awt.Color;
import java.awt.Font;

public class HText extends HStaticText implements HNavigable {
    public HText() {}
    public HText(String t) { super(t); }
    public HText(String t, int x, int y, int w, int h) { super(t, x, y, w, h); }
    public HText(String n, String f) { super(n); setTextContent(f, FOCUSED_STATE); }
    public HText(String n, String f, int x, int y, int w, int h) { super(n, x, y, w, h); setTextContent(f, FOCUSED_STATE); }
    public HText(String t, Font f, Color fg, Color bg, HTextLayoutManager m) { super(t, f, fg, bg, m); }
    public HText(String t, int x, int y, int w, int h, Font f, Color fg, Color bg, HTextLayoutManager m) { super(t, x, y, w, h, f, fg, bg, m); }
    public HText(String n, String t, Font f, Color fg, Color bg, HTextLayoutManager m) { super(n, f, fg, bg, m); setTextContent(t, FOCUSED_STATE); }
    public HText(String n, String t, int x, int y, int w, int h, Font f, Color fg, Color bg, HTextLayoutManager m) { super(n, x, y, w, h, f, fg, bg, m); setTextContent(t, FOCUSED_STATE); }
}
