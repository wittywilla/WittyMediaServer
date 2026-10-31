package org.havi.ui;

import java.awt.Dimension;
import java.awt.Graphics;
import java.awt.Image;
import java.awt.event.KeyEvent;
import java.util.List;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.CopyOnWriteArrayList;
import org.havi.ui.event.HActionEvent;
import org.havi.ui.event.HActionListener;
import org.havi.ui.event.HFocusEvent;
import org.havi.ui.event.HFocusListener;

/**
 * The base of every widget. A class has one parent and the navigable widgets sit on several branches, so what
 * they share lives here too: a widget that says it is HNavigable or HActionable only has to declare it.
 */
public class HVisible extends HComponent implements HState {
    public static final int NO_BACKGROUND_FILL = 0, BACKGROUND_FILL = 1;
    public static final int NO_DEFAULT_WIDTH = -1, NO_DEFAULT_HEIGHT = -1;
    public static final Dimension NO_DEFAULT_SIZE = new Dimension(NO_DEFAULT_WIDTH, NO_DEFAULT_HEIGHT);
    public static final int RESIZE_NONE = 0, RESIZE_PRESERVE_ASPECT = 1, RESIZE_ARBITRARY = 2;
    public static final int HALIGN_LEFT = 0, HALIGN_CENTER = 1, HALIGN_RIGHT = 2, HALIGN_JUSTIFY = 3, VALIGN_TOP = 0, VALIGN_CENTER = 4, VALIGN_BOTTOM = 8, VALIGN_JUSTIFY = 12;
    public static final int FIRST_CHANGE = 0, TEXT_CONTENT_CHANGE = 0, GRAPHIC_CONTENT_CHANGE = 1, ANIMATE_CONTENT_CHANGE = 2, CONTENT_CHANGE = 3, STATE_CHANGE = 4, CARET_POSITION_CHANGE = 5, ECHO_CHAR_CHANGE = 6, EDIT_MODE_CHANGE = 7, MIN_MAX_CHANGE = 8, THUMB_OFFSETS_CHANGE = 9, ORIENTATION_CHANGE = 10, TEXT_VALUE_CHANGE = 11, ITEM_VALUE_CHANGE = 12, ADJUSTMENT_VALUE_CHANGE = 13, LIST_CONTENT_CHANGE = 14, LIST_ICONSIZE_CHANGE = 15, LIST_LABELSIZE_CHANGE = 16, LIST_MULTISELECTION_CHANGE = 17, LIST_SCROLLPOSITION_CHANGE = 18, SIZE_CHANGE = 19, BORDER_CHANGE = 20, REPEAT_COUNT_CHANGE = 21, ANIMATION_POSITION_CHANGE = 22, LIST_SELECTION_CHANGE = 23, UNKNOWN_CHANGE = 24, LAST_CHANGE = 24;
    private static final HTextLayoutManager TEXT = new HDefaultTextLayoutManager();
    private final Object[] texts = new Object[8], pics = new Object[8], anims = new Object[8], custom = new Object[8];
    private volatile int state = NORMAL_STATE;
    private int bg = NO_BACKGROUND_FILL, resize = RESIZE_NONE, halign = HALIGN_CENTER, valign = VALIGN_CENTER;
    private volatile HLook look;
    private HTextLayoutManager layout = TEXT;
    private Dimension defaultSize = NO_DEFAULT_SIZE;
    private boolean borders = true;
    private final Map<Object, Object> lookData = new ConcurrentHashMap<Object, Object>();
    private final Map<Integer, HNavigable> moves = new ConcurrentHashMap<Integer, HNavigable>();
    private final List<HFocusListener> focus = new CopyOnWriteArrayList<HFocusListener>();
    private final List<HActionListener> acts = new CopyOnWriteArrayList<HActionListener>();
    private HSound gain, lose, action;
    private String command;

    public HVisible() {}
    public HVisible(HLook l) { look = l; }
    public HVisible(HLook l, int x, int y, int w, int h) { super(x, y, w, h); look = l; }
    private static int at(int s) { return (s - FIRST_STATE) & 7; }
    final void changed(int hint, Object data) {
        HLook l = look;
        if (l != null) l.widgetChanged(this, new HChangeData[] {new HChangeData(hint, data)});
        else repaint();
    }
    private void each(Object[] slot, Object v, int mask, int hint) {
        if (mask == ALL_STATES) { for (int i = 0; i < 8; i++) slot[i] = v; }
        else slot[at(mask)] = v;
        changed(hint, Integer.valueOf(mask));
    }
    public void setTextContent(String t, int mask) { each(texts, t, mask, TEXT_CONTENT_CHANGE); }
    public void setGraphicContent(Image i, int mask) { each(pics, i, mask, GRAPHIC_CONTENT_CHANGE); }
    public void setAnimateContent(Image[] a, int mask) { each(anims, a, mask, ANIMATE_CONTENT_CHANGE); }
    public void setContent(Object o, int mask) { each(custom, o, mask, CONTENT_CHANGE); }
    public String getTextContent(int s) { return (String) texts[at(s)]; }
    public Image getGraphicContent(int s) { return (Image) pics[at(s)]; }
    public Image[] getAnimateContent(int s) { return (Image[]) anims[at(s)]; }
    public Object getContent(int s) { return custom[at(s)]; }
    /** The longest animation this widget holds, without calling anything a disc could override. */
    final int frameCount() { int n = 0; for (int i = 0; i < 8; i++) if (anims[i] != null) n = Math.max(n, ((Image[]) anims[i]).length); return n; }
    public void setInteractionState(int s) {
        if (s < FIRST_STATE || s > LAST_STATE) return;
        int old = state;
        if (old == s) return;
        state = s;
        changed(STATE_CHANGE, Integer.valueOf(old));
    }
    public int getInteractionState() { return state; }
    final void setStateBit(int bit, boolean on) { int s = getInteractionState(); setInteractionState(on ? s | bit : s & ~bit); }
    public void setEnabled(boolean e) { super.setEnabled(e); setStateBit(DISABLED_STATE_BIT, !e); }
    public void setLook(HLook l) throws HInvalidLookException { look = l; repaint(); }
    public HLook getLook() { return look; }
    public void setTextLayoutManager(HTextLayoutManager m) { layout = m == null ? TEXT : m; repaint(); }
    public HTextLayoutManager getTextLayoutManager() { return layout; }
    public void setBackgroundMode(int m) { bg = m; repaint(); }
    public int getBackgroundMode() { return bg; }
    public void setResizeMode(int m) { resize = m; repaint(); }
    public int getResizeMode() { return resize; }
    public void setHorizontalAlignment(int a) { halign = a; repaint(); }
    public void setVerticalAlignment(int a) { valign = a; repaint(); }
    public int getHorizontalAlignment() { return halign; }
    public int getVerticalAlignment() { return valign; }
    public void setBordersEnabled(boolean e) { borders = e; changed(BORDER_CHANGE, Boolean.valueOf(!e)); }
    public boolean getBordersEnabled() { return borders; }
    public void setDefaultSize(Dimension d) { defaultSize = d == null ? NO_DEFAULT_SIZE : d; }
    public Dimension getDefaultSize() { return defaultSize; }
    public Object getLookData(Object k) { return k == null ? null : lookData.get(k); }
    public void setLookData(Object k, Object d) { if (k == null) return; if (d == null) lookData.remove(k); else lookData.put(k, d); }
    public boolean isOpaque() { HLook l = look; return l != null && l.isOpaque(this); }
    public Dimension getMinimumSize() { HLook l = look; return l != null ? l.getMinimumSize(this) : super.getMinimumSize(); }
    public Dimension getPreferredSize() { HLook l = look; return l != null ? l.getPreferredSize(this) : super.getPreferredSize(); }
    public Dimension getMaximumSize() { HLook l = look; return l != null ? l.getMaximumSize(this) : super.getMaximumSize(); }
    public void paint(Graphics g) {
        final HLook l = look;
        if (l == null) return;
        through(getMatte(), g, getWidth(), getHeight(), new Drawing() { public void draw(Graphics b) { l.showLook(b, HVisible.this, getInteractionState()); } });
    }
    public void update(Graphics g) { paint(g); }

    // HNavigable.
    public void setMove(int c, HNavigable t) { if (t == null) moves.remove(c); else moves.put(c, t); }
    public HNavigable getMove(int c) { return moves.get(c); }
    public void setFocusTraversal(HNavigable u, HNavigable d, HNavigable l, HNavigable r) {
        setMove(KeyEvent.VK_UP, u);
        setMove(KeyEvent.VK_DOWN, d);
        setMove(KeyEvent.VK_LEFT, l);
        setMove(KeyEvent.VK_RIGHT, r);
    }
    public boolean isSelected() { return (getInteractionState() & FOCUSED_STATE_BIT) != 0; }
    public void setGainFocusSound(HSound s) { gain = s; }
    public void setLoseFocusSound(HSound s) { lose = s; }
    public HSound getGainFocusSound() { return gain; }
    public HSound getLoseFocusSound() { return lose; }
    public void addHFocusListener(HFocusListener l) { if (l != null) focus.add(l); }
    public void removeHFocusListener(HFocusListener l) { focus.remove(l); }
    public int[] getNavigationKeys() {
        Integer[] all = moves.keySet().toArray(new Integer[0]);
        if (all.length == 0) return null;
        int[] k = new int[all.length];
        for (int i = 0; i < k.length; i++) k[i] = all[i];
        return k;
    }
    public void processHFocusEvent(HFocusEvent e) {
        if (e.getID() == HFocusEvent.FOCUS_GAINED) {
            if (!isSelected()) { setStateBit(FOCUSED_STATE_BIT, true); play(gain); }
            for (HFocusListener l : focus) l.focusGained(e);
        } else if (e.getID() == HFocusEvent.FOCUS_LOST) {
            if (isSelected()) { setStateBit(FOCUSED_STATE_BIT, false); play(lose); }
            for (HFocusListener l : focus) l.focusLost(e);
        }
    }
    static void play(HSound s) { if (s != null) s.play(); }

    // HActionable. The actioned look stays long enough for the viewer to see it.
    public void addHActionListener(HActionListener l) { if (l != null) acts.add(l); }
    public void removeHActionListener(HActionListener l) { acts.remove(l); }
    public void setActionCommand(String c) { command = c; }
    public String getActionCommand() { return command; }
    public void setActionSound(HSound s) { action = s; }
    public HSound getActionSound() { return action; }
    public void processHActionEvent(HActionEvent e) {
        if ((getInteractionState() & DISABLED_STATE_BIT) != 0) return;
        play(action);
        setStateBit(ACTIONED_STATE_BIT, true);
        fireAction(e);
        HEffect.after(150, new Runnable() { public void run() { setStateBit(ACTIONED_STATE_BIT, false); } });
    }
    final void fireAction(HActionEvent e) { for (HActionListener l : acts) l.actionPerformed(e); }
}
