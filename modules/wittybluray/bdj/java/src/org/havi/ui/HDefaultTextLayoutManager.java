package org.havi.ui;

import java.awt.FontMetrics;
import java.awt.Graphics;
import java.awt.Insets;
import java.util.ArrayList;
import java.util.List;

/** Lines broken at line breaks and, to fit the width, at spaces; placed by the widget's alignment. */
public class HDefaultTextLayoutManager implements HTextLayoutManager {
    public void render(String t, Graphics g, HVisible v, Insets i) {
        if (t == null) return;
        FontMetrics m = g.getFontMetrics();
        int left = i == null ? 0 : i.left, top = i == null ? 0 : i.top;
        int W = v.getWidth() - left - (i == null ? 0 : i.right), H = v.getHeight() - top - (i == null ? 0 : i.bottom);
        List<String> lines = lines(t, m, W);
        int total = lines.size() * m.getHeight(), a = v.getHorizontalAlignment(), b = v.getVerticalAlignment();
        int y = top + m.getAscent() + (b == HVisible.VALIGN_CENTER ? (H - total) / 2 : b == HVisible.VALIGN_BOTTOM ? H - total : 0);
        for (String s : lines) {
            int w = m.stringWidth(s);
            g.drawString(s, left + (a == HVisible.HALIGN_CENTER ? (W - w) / 2 : a == HVisible.HALIGN_RIGHT ? W - w : 0), y);
            y += m.getHeight();
        }
    }
    private static List<String> lines(String t, FontMetrics m, int width) {
        List<String> o = new ArrayList<String>();
        for (String part : t.split("\n", -1)) {
            StringBuilder s = new StringBuilder();
            for (String w : part.split(" ", -1)) {
                if (s.length() > 0 && width > 0 && m.stringWidth(s + " " + w) > width) {
                    o.add(s.toString());
                    s.setLength(0);
                } else if (s.length() > 0) {
                    s.append(' ');
                }
                s.append(w);
            }
            o.add(s.toString());
        }
        return o;
    }
    public java.awt.Dimension getMinimumSize(HVisible v) { return new java.awt.Dimension(0, 0); }
    public java.awt.Dimension getMaximumSize(HVisible v) { return new java.awt.Dimension(32767, 32767); }
    public java.awt.Dimension getPreferredSize(HVisible v) { return v.getSize(); }
}
