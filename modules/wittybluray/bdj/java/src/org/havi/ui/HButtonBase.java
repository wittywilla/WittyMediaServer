package org.havi.ui;

import org.havi.ui.event.HActionEvent;
import org.havi.ui.event.HActionListener;
import org.havi.ui.event.HFocusEvent;
import org.havi.ui.event.HFocusListener;

/** What the two kinds of button share. */
class HButtonBase extends HVisible implements HActionable {
    private final java.util.Map<Integer, HNavigable> moves = new java.util.HashMap<Integer, HNavigable>();
    private final java.util.List<HActionListener> acts = new java.util.ArrayList<HActionListener>();
    private final java.util.List<HFocusListener> focus = new java.util.ArrayList<HFocusListener>();
    private String command = "";
    private HSound gain, lose, action;
    HButtonBase(HLook l) { super(l); }
    public void setMove(int c, HNavigable t) { moves.put(c, t); }
    public HNavigable getMove(int c) { return moves.get(c); }
    public void setFocusTraversal(HNavigable u, HNavigable d, HNavigable l, HNavigable r) {
        setMove(java.awt.event.KeyEvent.VK_UP, u);
        setMove(java.awt.event.KeyEvent.VK_DOWN, d);
        setMove(java.awt.event.KeyEvent.VK_LEFT, l);
        setMove(java.awt.event.KeyEvent.VK_RIGHT, r);
    }
    public boolean isSelected() { return (getInteractionState() & FOCUSED_STATE_BIT) != 0; }
    public void setGainFocusSound(HSound s) { gain = s; }
    public void setLoseFocusSound(HSound s) { lose = s; }
    public HSound getGainFocusSound() { return gain; }
    public HSound getLoseFocusSound() { return lose; }
    public void addHFocusListener(HFocusListener l) { focus.add(l); }
    public void removeHFocusListener(HFocusListener l) { focus.remove(l); }
    public int[] getNavigationKeys() { int[] k = new int[moves.size()]; int i = 0; for (Integer c : moves.keySet()) k[i++] = c; return k; }
    public void processHFocusEvent(HFocusEvent e) { for (HFocusListener l : focus) { if (e.getID() == HFocusEvent.FOCUS_GAINED) l.focusGained(e); else l.focusLost(e); } }
    public void addHActionListener(HActionListener l) { acts.add(l); }
    public void removeHActionListener(HActionListener l) { acts.remove(l); }
    public void setActionCommand(String c) { command = c; }
    public String getActionCommand() { return command; }
    public void setActionSound(HSound s) { action = s; }
    public HSound getActionSound() { return action; }
    public void processHActionEvent(HActionEvent e) { for (HActionListener l : acts) l.actionPerformed(e); }
}
