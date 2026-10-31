package org.havi.ui;

import java.util.ArrayList;
import java.util.List;

/** Toggle buttons of which at most one is on, and, when forced, always one. */
public class HToggleGroup {
    private final List<HToggleButton> buttons = new ArrayList<HToggleButton>();
    private HToggleButton current;
    private boolean forced, enabled = true;
    public HToggleGroup() {}
    public synchronized HToggleButton getCurrent() { return current; }
    public void setCurrent(HToggleButton b) {
        if (b != null) { b.setSwitchableState(true); return; }
        HToggleButton c = getCurrent();
        if (c != null && !getForcedSelection()) c.setSwitchableState(false);
    }
    public synchronized void setForcedSelection(boolean f) {
        forced = f;
        if (f && current == null && !buttons.isEmpty()) turn(buttons.get(0), true);
    }
    public synchronized boolean getForcedSelection() { return forced; }
    public synchronized void setEnabled(boolean e) {
        enabled = e;
        for (HToggleButton b : buttons) b.setEnabled(e);
    }
    public synchronized boolean isEnabled() { return enabled; }
    protected synchronized void add(HToggleButton b) {
        if (b == null || buttons.contains(b)) return;
        buttons.add(b);
        if (b.getToggleGroup() != this) b.setToggleGroup(this);
        if (b.getSwitchableState()) turn(b, true);
        else if (forced && current == null) turn(b, true);
    }
    protected synchronized void remove(HToggleButton b) {
        if (!buttons.remove(b)) return;
        if (b.getToggleGroup() == this) b.removeToggleGroup();
        if (current == b) {
            current = null;
            if (forced && !buttons.isEmpty()) turn(buttons.get(0), true);
        }
    }
    synchronized void turn(HToggleButton b, boolean on) {
        if (on) {
            if (current != null && current != b) current.raw(false);
            current = b;
            b.raw(true);
        } else if (current == b) {
            if (forced && buttons.size() > 0) return;
            current = null;
            b.raw(false);
        } else {
            b.raw(false);
        }
    }
}
