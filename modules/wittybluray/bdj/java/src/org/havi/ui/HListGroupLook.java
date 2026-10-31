package org.havi.ui;

import java.awt.Color;
import java.awt.Dimension;
import java.awt.FontMetrics;
import java.awt.Graphics;
import java.awt.Image;

/** The list's elements from the scroll position on: icon, label, a tint on the selected ones and a frame on the current one. */
public class HListGroupLook extends HDefaultLook {
    public void renderBorders(Graphics g, HVisible v, int state) { HTextLook.focusRing(g, v, state); }
    public void renderVisible(Graphics g, HVisible v, int state) {
        if (!(v instanceof HListGroup)) { super.renderVisible(g, v, state); return; }
        HListGroup l = (HListGroup) v;
        HListElement[] items = l.getListContent();
        if (items == null) return;
        Color fg = v.getForeground() != null ? v.getForeground() : Color.white;
        if (v.getFont() != null) g.setFont(v.getFont());
        FontMetrics m = g.getFontMetrics();
        Dimension ic = l.getIconSize(), la = l.getLabelSize();
        boolean tall = l.getOrientation() == HOrientable.ORIENT_TOP_TO_BOTTOM || l.getOrientation() == HOrientable.ORIENT_BOTTOM_TO_TOP;
        int iw = Math.max(0, ic.width), ih = Math.max(0, ic.height);
        int lh = la.height > 0 ? la.height : m.getHeight() + 4;
        int size = tall ? Math.max(ih, lh) : (la.width > 0 ? la.width : v.getWidth() / Math.max(1, items.length)) + iw;
        int room = tall ? v.getHeight() : v.getWidth();
        int current = l.getCurrentIndex();
        for (int i = Math.max(0, l.getScrollPosition()), at = 0; i < items.length && at < room; i++, at += Math.max(1, size)) {
            int x = tall ? 0 : at, y = tall ? at : 0, w = tall ? v.getWidth() : size, h = tall ? size : v.getHeight();
            if (l.isItemSelected(i)) {
                g.setColor(new Color(fg.getRed(), fg.getGreen(), fg.getBlue(), 64));
                g.fillRect(x, y, w, h);
            }
            g.setColor(fg);
            if (i == current && (state & HState.FOCUSED_STATE_BIT) != 0) g.drawRect(x, y, w - 1, h - 1);
            Image pic = items[i].getIcon();
            if (pic != null && iw > 0) g.drawImage(pic, x + 2, y + (h - ih) / 2, iw, ih, v);
            String t = items[i].getLabel();
            if (t != null) g.drawString(t, x + iw + 4, y + (h + m.getAscent() - m.getDescent()) / 2);
        }
    }
    /** How many elements fit, for scrolling by the page. */
    public int getNumVisible(HVisible v) {
        if (!(v instanceof HListGroup) || v.getFont() == null) return 1;
        Dimension la = ((HListGroup) v).getLabelSize();
        int h = la.height > 0 ? la.height : witty.shim.Toolkit.getDefaultToolkit().getFontMetrics(v.getFont()).getHeight() + 4;
        return Math.max(1, v.getHeight() / Math.max(1, h));
    }
}
