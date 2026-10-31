package org.havi.ui;

import java.util.List;
import org.havi.ui.event.HAdjustmentEvent;
import org.havi.ui.event.HAdjustmentListener;

public class HRangeValue extends HRange implements HAdjustmentValue {
    private final List<HAdjustmentListener> ls = new java.util.concurrent.CopyOnWriteArrayList<HAdjustmentListener>();
    private int unit = 1, block = 10;
    private boolean adjusting;
    private HSound sound;
    public HRangeValue() {}
    public HRangeValue(int o, int mn, int mx, int v) { super(o, mn, mx, v); }
    public HRangeValue(int o, int mn, int mx, int v, int x, int y, int w, int h) { super(o, mn, mx, v, x, y, w, h); }
    public void setUnitIncrement(int n) { if (n > 0) unit = n; }
    public int getUnitIncrement() { return unit; }
    public void setBlockIncrement(int n) { if (n > 0) block = n; }
    public int getBlockIncrement() { return block; }
    public void addAdjustmentListener(HAdjustmentListener l) { if (l != null) ls.add(l); }
    public void removeAdjustmentListener(HAdjustmentListener l) { ls.remove(l); }
    public void setAdjustmentSound(HSound s) { sound = s; }
    public HSound getAdjustmentSound() { return sound; }
    public boolean getAdjustMode() { return adjusting; }
    public void setAdjustMode(boolean on) { adjusting = on; setStateBit(ACTIONED_STATE_BIT, on); }
    public void processHAdjustmentEvent(HAdjustmentEvent e) {
        if ((getInteractionState() & DISABLED_STATE_BIT) != 0) return;
        switch (e.getID()) {
            case HAdjustmentEvent.ADJUST_START_CHANGE: setAdjustMode(true); break;
            case HAdjustmentEvent.ADJUST_END_CHANGE: setAdjustMode(false); break;
            case HAdjustmentEvent.ADJUST_LESS: setValue(getValue() - unit); break;
            case HAdjustmentEvent.ADJUST_MORE: setValue(getValue() + unit); break;
            case HAdjustmentEvent.ADJUST_PAGE_LESS: setValue(getValue() - block); break;
            case HAdjustmentEvent.ADJUST_PAGE_MORE: setValue(getValue() + block); break;
            default: return;
        }
        play(sound);
        for (HAdjustmentListener l : ls) l.valueChanged(e);
    }
}
