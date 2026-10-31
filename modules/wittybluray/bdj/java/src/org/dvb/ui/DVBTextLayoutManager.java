package org.dvb.ui;

import java.awt.Dimension;
import java.awt.FontMetrics;
import java.awt.Graphics;
import java.awt.Insets;
import java.util.ArrayList;
import java.util.List;
import org.havi.ui.HVisible;

/** Lays text out in a box: line breaks and tags, word wrap, alignment, tabs and spacing, drawn with the given Graphics. */
public class DVBTextLayoutManager implements org.havi.ui.HTextLayoutManager {
    public static final int LINE_ORIENTATION_HORIZONTAL = 0, LINE_ORIENTATION_VERTICAL = 1;
    public static final int START_CORNER_UPPER_LEFT = 0, START_CORNER_UPPER_RIGHT = 1, START_CORNER_LOWER_LEFT = 2, START_CORNER_LOWER_RIGHT = 3;
    public static final int HORIZONTAL_START_ALIGN = 0, HORIZONTAL_CENTER = 1, HORIZONTAL_END_ALIGN = 2;
    public static final int VERTICAL_START_ALIGN = 0, VERTICAL_CENTER = 1, VERTICAL_END_ALIGN = 2;

    private int orient = LINE_ORIENTATION_HORIZONTAL, corner = START_CORNER_UPPER_LEFT, halign = HORIZONTAL_START_ALIGN, valign = VERTICAL_START_ALIGN;
    private int lineSpace, letterSpace, tabSpace;
    private boolean wrap = true, overflowing;
    private Insets insets = new Insets(0, 0, 0, 0);
    private final List<TextOverflowListener> listeners = new java.util.concurrent.CopyOnWriteArrayList<TextOverflowListener>();

    public DVBTextLayoutManager() {}
    public DVBTextLayoutManager(int linesOrientation, int startCorner, int horizontalAlign, int verticalAlign, int lineSpace, int letterSpace, int horizontalTabSpace, boolean wrap) {
        orient = linesOrientation;
        corner = startCorner;
        halign = horizontalAlign;
        valign = verticalAlign;
        this.lineSpace = lineSpace;
        this.letterSpace = letterSpace;
        tabSpace = horizontalTabSpace;
        this.wrap = wrap;
    }
    public int getLineOrientation() { return orient; }
    public void setLineOrientation(int v) { orient = v; }
    public int getStartCorner() { return corner; }
    public void setStartCorner(int v) { corner = v; }
    public int getHorizontalAlign() { return halign; }
    public void setHorizontalAlign(int v) { halign = v; }
    public int getVerticalAlign() { return valign; }
    public void setVerticalAlign(int v) { valign = v; }
    public int getLineSpace() { return lineSpace; }
    public void setLineSpace(int v) { lineSpace = v; }
    public int getLetterSpace() { return letterSpace; }
    public void setLetterSpace(int v) { letterSpace = v; }
    public int getHorizontalTabSpace() { return tabSpace; }
    public void setHorizontalTabSpace(int v) { tabSpace = v; }
    public boolean isWrap() { return wrap; }
    public void setWrap(boolean v) { wrap = v; }
    public Insets getInsets() { return (Insets) insets.clone(); }
    public void setInsets(Insets i) { insets = i == null ? new Insets(0, 0, 0, 0) : (Insets) i.clone(); }
    public void addTextOverflowListener(TextOverflowListener l) { if (l != null) listeners.add(l); }
    public void removeTextOverflowListener(TextOverflowListener l) { listeners.remove(l); }

    /** The text without its tags: p and br end a line, the rest is dropped. */
    static String plain(String s) {
        if (s == null) return "";
        StringBuilder b = new StringBuilder(s.length());
        for (int i = 0; i < s.length(); i++) {
            char c = s.charAt(i);
            if (c == '<') {
                int e = s.indexOf('>', i);
                if (e > 0) {
                    String t = s.substring(i + 1, e).trim().toLowerCase();
                    if (t.equals("p") || t.equals("br") || t.equals("br/") || t.equals("/p")) b.append('\n');
                    i = e;
                    continue;
                }
            } else if (c == '&') {
                int e = s.indexOf(';', i);
                if (e > 0 && e - i <= 5) {
                    String t = s.substring(i + 1, e);
                    String r = t.equals("lt") ? "<" : t.equals("gt") ? ">" : t.equals("amp") ? "&" : t.equals("quot") ? "\"" : t.equals("nbsp") ? " " : null;
                    if (r != null) {
                        b.append(r);
                        i = e;
                        continue;
                    }
                }
            } else if (c == '\r') {
                continue;
            }
            b.append(c);
        }
        return b.toString();
    }

    private int span(FontMetrics m, String s, int from) {
        int x = from;
        for (int i = 0; i < s.length(); i++) {
            char c = s.charAt(i);
            if (c == '\t') {
                int t = tabSpace > 0 ? tabSpace : m.charWidth(' ') * 4;
                x = (x / t + 1) * t;
            } else {
                x += m.charWidth(c) + letterSpace;
            }
        }
        return x - from;
    }

    private List<String> lines(String text, FontMetrics m, int width) {
        List<String> out = new ArrayList<String>();
        String[] paras = plain(text).split("\n", -1);
        for (int p = 0; p < paras.length; p++) {
            String para = paras[p];
            if (!wrap || span(m, para, 0) <= width) {
                out.add(para);
                continue;
            }
            StringBuilder cur = new StringBuilder();
            String[] words = para.split(" ", -1);
            for (int w = 0; w < words.length; w++) {
                String trial = cur.length() == 0 ? words[w] : cur + " " + words[w];
                if (cur.length() > 0 && span(m, trial, 0) > width) {
                    out.add(cur.toString());
                    cur.setLength(0);
                }
                cur.append(cur.length() == 0 ? words[w] : " " + words[w]);
                while (span(m, cur.toString(), 0) > width && cur.length() > 1) { // one word wider than the box: break it
                    int n = cur.length() - 1;
                    while (n > 1 && span(m, cur.substring(0, n), 0) > width) n--;
                    out.add(cur.substring(0, n));
                    cur.delete(0, n);
                }
            }
            out.add(cur.toString());
        }
        return out;
    }

    private void draw(Graphics g, FontMetrics m, String s, int x, int base) {
        if (letterSpace == 0 && s.indexOf('\t') < 0) {
            g.drawString(s, x, base);
            return;
        }
        int at = x;
        for (int i = 0; i < s.length(); i++) {
            char c = s.charAt(i);
            if (c == '\t') {
                int t = tabSpace > 0 ? tabSpace : m.charWidth(' ') * 4;
                at = x + ((at - x) / t + 1) * t;
            } else {
                g.drawString(String.valueOf(c), at, base);
                at += m.charWidth(c) + letterSpace;
            }
        }
    }

    public void render(String markedUpString, Graphics g, HVisible v, Insets ins) {
        if (g == null || v == null) return;
        Insets in = ins != null ? ins : insets;
        FontMetrics m = g.getFontMetrics();
        int w = v.getWidth() - in.left - in.right, h = v.getHeight() - in.top - in.bottom;
        if (w <= 0 || h <= 0) return;
        List<String> ls = lines(markedUpString, m, w);
        int step = m.getHeight() + lineSpace;
        int fit = Math.max(1, (h + lineSpace) / step);
        boolean over = ls.size() > fit;
        if (over && ls.size() > fit) ls = ls.subList(0, fit);
        int block = ls.size() * step - lineSpace;
        boolean top = corner == START_CORNER_UPPER_LEFT || corner == START_CORNER_UPPER_RIGHT;
        boolean left = corner == START_CORNER_UPPER_LEFT || corner == START_CORNER_LOWER_LEFT;
        int y0;
        int va = valign;
        if (!top && va != VERTICAL_CENTER) va = va == VERTICAL_START_ALIGN ? VERTICAL_END_ALIGN : VERTICAL_START_ALIGN;
        y0 = va == VERTICAL_CENTER ? in.top + (h - block) / 2 : va == VERTICAL_END_ALIGN ? in.top + h - block : in.top;
        java.awt.Shape oldClip = g.getClip();
        g.clipRect(in.left, in.top, w, h);
        try {
            for (int i = 0; i < ls.size(); i++) {
                String s = ls.get(top ? i : ls.size() - 1 - i);
                int sw = span(m, s, 0), x;
                int ha = halign;
                if (!left && ha != HORIZONTAL_CENTER) ha = ha == HORIZONTAL_START_ALIGN ? HORIZONTAL_END_ALIGN : HORIZONTAL_START_ALIGN;
                x = ha == HORIZONTAL_CENTER ? in.left + (w - sw) / 2 : ha == HORIZONTAL_END_ALIGN ? in.left + w - sw : in.left;
                draw(g, m, s, x, y0 + i * step + m.getAscent());
            }
        } finally {
            g.setClip(oldClip);
        }
        if (over && !overflowing) {
            overflowing = true;
            tell();
        } else if (!over) {
            overflowing = false;
        }
    }

    private void tell() {
        if (listeners.isEmpty()) return;
        final java.util.EventObject e = new java.util.EventObject(this);
        Thread t = new Thread(new Runnable() {
            public void run() { for (TextOverflowListener l : listeners) { try { l.textOverflow(e); } catch (Throwable x) {} } }
        }, "text-overflow");
        t.setDaemon(true);
        t.start();
    }

    public Dimension getMinimumSize(HVisible v) { return new Dimension(in(0), in(1)); }
    public Dimension getMaximumSize(HVisible v) { return new Dimension(32767, 32767); }
    public Dimension getPreferredSize(HVisible v) {
        if (v == null) return new Dimension(0, 0);
        java.awt.Font f = v.getFont();
        if (f == null) return v.getSize();
        FontMetrics m = v.getFontMetrics(f);
        String t = v.getTextContent(HVisible.NORMAL_STATE);
        if (m == null || t == null) return v.getSize();
        List<String> ls = lines(t, m, Integer.MAX_VALUE / 2);
        int wid = 0;
        for (int i = 0; i < ls.size(); i++) wid = Math.max(wid, span(m, ls.get(i), 0));
        return new Dimension(wid + insets.left + insets.right, ls.size() * (m.getHeight() + lineSpace) - lineSpace + insets.top + insets.bottom);
    }
    private int in(int axis) { return axis == 0 ? insets.left + insets.right : insets.top + insets.bottom; }
}
