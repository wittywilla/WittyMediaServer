package org.havi.ui;

import java.awt.Color;
import java.awt.Dimension;
import java.awt.FontMetrics;
import java.awt.Graphics;
import java.awt.Image;
import java.awt.Insets;

/** Draws a visible's picture and text: the looks disc authors get without writing their own build on this. */
public class HDefaultLook implements HExtendedLook {
    public void showLook(Graphics g, HVisible v, int state) {
        fillBackground(g, v, state);
        renderBorders(g, v, state);
        renderVisible(g, v, state);
    }
    public void fillBackground(Graphics g, HVisible v, int state) {
        Color c = v.getBackground();
        if (v.getBackgroundMode() != HVisible.BACKGROUND_FILL || c == null) return;
        g.setColor(c);
        g.fillRect(0, 0, v.getWidth(), v.getHeight());
    }
    public void renderBorders(Graphics g, HVisible v, int state) {}
    public void renderVisible(Graphics g, HVisible v, int state) {
        drawGraphic(g, v, graphicFor(v, state));
        drawText(g, v, textFor(v, state));
    }
    public void widgetChanged(HVisible v, HChangeData[] c) { v.repaint(); }
    public Dimension getMinimumSize(HVisible v) { Dimension d = content(v); return d != null ? d : new Dimension(0, 0); }
    public Dimension getPreferredSize(HVisible v) {
        Dimension d = v.getDefaultSize();
        if (d != null && d.width != HVisible.NO_DEFAULT_WIDTH && d.height != HVisible.NO_DEFAULT_HEIGHT) return new Dimension(d);
        d = content(v);
        return d != null ? d : v.getSize();
    }
    public Dimension getMaximumSize(HVisible v) { return new Dimension(32767, 32767); }
    public boolean isOpaque(HVisible v) { Color c = v.getBackground(); return v.getBackgroundMode() == HVisible.BACKGROUND_FILL && c != null && c.getAlpha() == 255; }
    public Insets getInsets(HVisible v) { return new Insets(0, 0, 0, 0); }

    /** How big the content is, or null when there is none. */
    Dimension content(HVisible v) {
        int w = -1, h = -1;
        for (int s = HState.FIRST_STATE; s <= HState.LAST_STATE; s++) {
            Image i = v.getGraphicContent(s);
            if (i != null) { w = Math.max(w, i.getWidth(null)); h = Math.max(h, i.getHeight(null)); }
            String t = v.getTextContent(s);
            if (t != null && v.getFont() != null) {
                FontMetrics m = witty.shim.Toolkit.getDefaultToolkit().getFontMetrics(v.getFont());
                w = Math.max(w, m.stringWidth(t));
                h = Math.max(h, m.getHeight());
            }
        }
        return w < 0 || h < 0 ? null : new Dimension(w, h);
    }
    /** A state with no content of its own borrows from the nearest one. */
    static int nearer(int s) {
        switch (s) {
            case HState.FOCUSED_STATE: case HState.DISABLED_STATE: return HState.NORMAL_STATE;
            case HState.ACTIONED_STATE: case HState.DISABLED_FOCUSED_STATE: case HState.ACTIONED_FOCUSED_STATE: return HState.FOCUSED_STATE;
            case HState.DISABLED_ACTIONED_STATE: return HState.ACTIONED_STATE;
            case HState.DISABLED_ACTIONED_FOCUSED_STATE: return HState.ACTIONED_FOCUSED_STATE;
            default: return -1;
        }
    }
    static Image graphicFor(HVisible v, int state) {
        for (int s = state; s >= 0; s = nearer(s)) { Image i = v.getGraphicContent(s); if (i != null) return i; }
        return null;
    }
    static String textFor(HVisible v, int state) {
        for (int s = state; s >= 0; s = nearer(s)) { String t = v.getTextContent(s); if (t != null) return t; }
        return null;
    }
    static Image[] framesFor(HVisible v, int state) {
        for (int s = state; s >= 0; s = nearer(s)) { Image[] a = v.getAnimateContent(s); if (a != null && a.length > 0) return a; }
        return null;
    }
    static void drawGraphic(Graphics g, HVisible v, Image i) {
        if (i == null) return;
        int w = i.getWidth(v), h = i.getHeight(v), W = v.getWidth(), H = v.getHeight();
        if (w <= 0 || h <= 0) return;
        int mode = v.getResizeMode();
        if (mode == HVisible.RESIZE_ARBITRARY) { g.drawImage(i, 0, 0, W, H, v); return; }
        if (mode == HVisible.RESIZE_PRESERVE_ASPECT) {
            double f = Math.min(W / (double) w, H / (double) h);
            w = Math.max(1, (int) (w * f));
            h = Math.max(1, (int) (h * f));
        }
        int a = v.getHorizontalAlignment(), b = v.getVerticalAlignment();
        int x = a == HVisible.HALIGN_RIGHT ? W - w : a == HVisible.HALIGN_CENTER ? (W - w) / 2 : 0;
        int y = b == HVisible.VALIGN_BOTTOM ? H - h : b == HVisible.VALIGN_CENTER ? (H - h) / 2 : 0;
        if (mode == HVisible.RESIZE_PRESERVE_ASPECT) g.drawImage(i, x, y, w, h, v);
        else g.drawImage(i, x, y, v);
    }
    void drawText(Graphics g, HVisible v, String t) {
        if (t == null) return;
        Color c = v.getForeground();
        g.setColor(c != null ? c : Color.white);
        if (v.getFont() != null) g.setFont(v.getFont());
        v.getTextLayoutManager().render(t, g, v, getInsets(v));
    }
}
