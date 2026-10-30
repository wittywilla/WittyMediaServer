package org.havi.ui;

import java.awt.Component;
import java.awt.Container;

public class HContainer extends Container implements HMatteLayer, HComponentOrdering, org.dvb.ui.TestOpacity {
    private HMatte matte;
    public HContainer() { setLayout(null); }
    public HContainer(int x, int y, int w, int h) { this(); setBounds(x, y, w, h); }
    public void setMatte(HMatte m) { matte = m; }
    public HMatte getMatte() { return matte; }
    public boolean isOpaque() { return false; }
    public void group() {}
    public void ungroup() {}
    public boolean isGrouped() { return false; }
    public Component addBefore(Component c, Component b) { return order(c, b, 0); }
    public Component addAfter(Component c, Component b) { return order(c, b, 1); }
    private Component order(Component c, Component b, int d) {
        Component[] all = getComponents();
        for (int i = 0; i < all.length; i++) if (all[i] == b) { remove(c); add(c, Math.min(i + d, getComponentCount())); return c; }
        return null;
    }
    public boolean popToFront(Component c) { remove(c); add(c, 0); return true; }
    public boolean pushToBack(Component c) { remove(c); add(c); return true; }
    public boolean pop(Component c) { return popToFront(c); }
    public boolean push(Component c) { return pushToBack(c); }
    public boolean popInFrontOf(Component m, Component n) { return addBefore(m, n) != null; }
    public boolean pushBehind(Component m, Component n) { return addAfter(m, n) != null; }
}
