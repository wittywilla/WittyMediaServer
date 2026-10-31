package org.havi.ui;

import java.awt.Component;
import java.awt.Container;
import java.awt.Graphics;
import java.awt.Image;
import witty.bdj.Keys;
import witty.bdj.Plane;

public class HContainer extends Container implements HMatteLayer, HComponentOrdering, org.dvb.ui.TestOpacity {
    private HMatte matte;
    private boolean grouped;
    public HContainer() { setLayout(null); }
    public HContainer(int x, int y, int w, int h) { this(); setBounds(x, y, w, h); }
    public void setMatte(HMatte m) throws HMatteException { matte = m; Plane.invalidate(); }
    public HMatte getMatte() { return matte; }
    public boolean isOpaque() { return false; }
    public boolean isDoubleBuffered() { return false; }
    public void group() { grouped = true; }
    public void ungroup() { grouped = false; }
    public boolean isGrouped() { return grouped; }
    public void repaint(long t, int x, int y, int w, int h) { Plane.invalidate(); }
    public void requestFocus() { HComponent.focus(this); }
    public boolean requestFocusInWindow() { HComponent.focus(this); return true; }
    public boolean hasFocus() { return Keys.getFocus() == this; }
    public boolean isFocusOwner() { return hasFocus(); }
    public Image createImage(int w, int h) { return Plane.makeImage(w, h); }
    protected void addImpl(Component c, Object k, int i) { super.addImpl(c, k, i); Plane.invalidate(); }
    public void remove(int i) { super.remove(i); Plane.invalidate(); }
    public void remove(Component c) { super.remove(c); Plane.invalidate(); }
    public void paint(final Graphics g) {
        HMatte m = matte;
        if (m == null) super.paint(g);
        else HComponent.through(m, g, getWidth(), getHeight(), new HComponent.Drawing() { public void draw(Graphics b) { HContainer.super.paint(b); } });
    }
    public void update(Graphics g) { paint(g); }
    public Component addBefore(Component c, Component b) { return place(this, c, b, 0); }
    public Component addAfter(Component c, Component b) { return place(this, c, b, 1); }
    public boolean popToFront(Component c) { return shift(this, c, 0); }
    public boolean pushToBack(Component c) { return shift(this, c, Integer.MAX_VALUE); }
    public boolean pop(Component c) { return step(this, c, -1); }
    public boolean push(Component c) { return step(this, c, 1); }
    public boolean popInFrontOf(Component m, Component n) { return index(this, m) >= 0 && place(this, m, n, 0) != null; }
    public boolean pushBehind(Component m, Component n) { return index(this, m) >= 0 && place(this, m, n, 1) != null; }

    // The ordering, for here and for HScene: front of the list is front of the screen.
    static int index(Container p, Component c) {
        if (c == null) return -1;
        Component[] all = p.getComponents();
        for (int i = 0; i < all.length; i++) if (all[i] == c) return i;
        return -1;
    }
    static Component place(Container p, Component c, Component ref, int behind) {
        if (c == null || c == ref) return null;
        synchronized (p.getTreeLock()) {
            int r = index(p, ref), at = index(p, c);
            if (r < 0) return null;
            if (at < 0) {
                p.add(c, r + behind);
            } else {
                p.setComponentZOrder(c, at < r ? r + behind - 1 : r + behind);
            }
        }
        Plane.invalidate();
        return c;
    }
    static boolean shift(Container p, Component c, int to) {
        synchronized (p.getTreeLock()) {
            if (index(p, c) < 0) return false;
            p.setComponentZOrder(c, Math.min(to, p.getComponentCount() - 1));
        }
        Plane.invalidate();
        return true;
    }
    static boolean step(Container p, Component c, int by) {
        synchronized (p.getTreeLock()) {
            int at = index(p, c);
            return at >= 0 && shift(p, c, Math.max(0, at + by));
        }
    }
}
