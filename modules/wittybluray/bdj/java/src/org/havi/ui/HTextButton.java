package org.havi.ui;

import java.awt.Color;
import java.awt.Font;

public class HTextButton extends HText implements HActionable {
    public HTextButton() {}
    public HTextButton(String t) { super(t); }
    public HTextButton(String t, int x, int y, int w, int h) { super(t, x, y, w, h); }
    public HTextButton(String t, Font f, Color fg, Color bg, HTextLayoutManager m) { super(t, f, fg, bg, m); }
    public HTextButton(String t, int x, int y, int w, int h, Font f, Color fg, Color bg, HTextLayoutManager m) { super(t, x, y, w, h, f, fg, bg, m); }
}
