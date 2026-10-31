package org.havi.ui;

import java.awt.Image;

public class HToggleButton extends HGraphicButton implements HSwitchable {
    private HToggleGroup group;
    private HSound unset;
    public HToggleButton() {}
    public HToggleButton(Image i) { super(i); }
    public HToggleButton(Image i, int x, int y, int w, int h) { super(i, x, y, w, h); }
    public HToggleButton(Image i, int x, int y, int w, int h, boolean on) { super(i, x, y, w, h); setSwitchableState(on); }
    public HToggleButton(Image i, int x, int y, int w, int h, boolean on, HToggleGroup g) { this(i, x, y, w, h, on); setToggleGroup(g); }
    public HToggleButton(Image i, boolean on, HToggleGroup g) { super(i); setSwitchableState(on); setToggleGroup(g); }
    public HToggleButton(Image n, Image f, Image a, Image s, boolean on) { super(n, f, a); setGraphicContent(s, DISABLED_ACTIONED_STATE); setSwitchableState(on); }
    public HToggleButton(Image n, Image f, Image a, Image s, int x, int y, int w, int h, boolean on) { super(n, f, a, x, y, w, h); setGraphicContent(s, DISABLED_ACTIONED_STATE); setSwitchableState(on); }
    public HToggleButton(Image n, Image f, Image a, Image s, boolean on, HToggleGroup g) { this(n, f, a, s, on); setToggleGroup(g); }
    public HToggleButton(Image n, Image f, Image a, Image s, int x, int y, int w, int h, boolean on, HToggleGroup g) { this(n, f, a, s, x, y, w, h, on); setToggleGroup(g); }
    public void setToggleGroup(HToggleGroup g) {
        if (group == g) return;
        removeToggleGroup();
        group = g;
        if (g != null) g.add(this);
    }
    public HToggleGroup getToggleGroup() { return group; }
    public void removeToggleGroup() {
        HToggleGroup g = group;
        group = null;
        if (g != null) g.remove(this);
    }
    public boolean getSwitchableState() { return (getInteractionState() & ACTIONED_STATE_BIT) != 0; }
    public void setSwitchableState(boolean on) {
        HToggleGroup g = group;
        if (g != null) g.turn(this, on);
        else raw(on);
    }
    final void raw(boolean on) { setStateBit(ACTIONED_STATE_BIT, on); }
    public void setUnsetActionSound(HSound s) { unset = s; }
    public HSound getUnsetActionSound() { return unset; }
    public void processHActionEvent(org.havi.ui.event.HActionEvent e) {
        if ((getInteractionState() & DISABLED_STATE_BIT) != 0) return;
        setSwitchableState(!getSwitchableState());
        play(getSwitchableState() ? getActionSound() : unset);
        fireAction(e);
    }
}
