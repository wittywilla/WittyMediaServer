package org.havi.ui;

import java.awt.Color;
import java.awt.Graphics;

/** A track with a thumb: filled up to the value for a slider, a sliding block for a scrollbar. */
public class HRangeLook extends HDefaultLook {
    public void renderBorders(Graphics g, HVisible v, int state) { HTextLook.focusRing(g, v, state); }
    public void renderVisible(Graphics g, HVisible v, int state) {
        if (!(v instanceof HStaticRange)) { super.renderVisible(g, v, state); return; }
        HStaticRange r = (HStaticRange) v;
        Color c = v.getForeground();
        g.setColor(c != null ? c : Color.white);
        int W = v.getWidth(), H = v.getHeight(), o = r.getOrientation(), span = Math.max(1, r.getMaxValue() - r.getMinValue());
        boolean flat = o == HOrientable.ORIENT_LEFT_TO_RIGHT || o == HOrientable.ORIENT_RIGHT_TO_LEFT;
        boolean back = o == HOrientable.ORIENT_RIGHT_TO_LEFT || o == HOrientable.ORIENT_BOTTOM_TO_TOP;
        int L = flat ? W : H, a = (r.getValue() - r.getMinValue()) * L / span, b = a;
        if (r.getBehavior() == HStaticRange.SCROLLBAR_BEHAVIOR) {
            a = (r.getValue() - r.getThumbMinOffset() - r.getMinValue()) * L / span;
            b = (r.getValue() + r.getThumbMaxOffset() - r.getMinValue()) * L / span;
        } else {
            b = a;
            a = 0;
        }
        if (back) { int t = L - b; b = L - a; a = t; }
        if (flat) g.fillRect(a, 0, Math.max(2, b - a), H);
        else g.fillRect(0, a, W, Math.max(2, b - a));
    }
}
