package org.havi.ui;

import java.awt.Dimension;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Collections;
import java.util.IdentityHashMap;
import java.util.List;
import java.util.Set;
import org.havi.ui.event.HItemEvent;
import org.havi.ui.event.HItemListener;

public class HListGroup extends HVisible implements HItemValue, HOrientable {
    public static final int ADD_INDEX_END = -1, ITEM_NOT_FOUND = -1, DEFAULT_ICON_WIDTH = -1, DEFAULT_ICON_HEIGHT = -1, DEFAULT_LABEL_WIDTH = -1, DEFAULT_LABEL_HEIGHT = -1;
    private static HListGroupLook look = new HListGroupLook();
    public static void setDefaultLook(HListGroupLook l) { look = l; }
    public static HListGroupLook getDefaultLook() { return look; }
    private final List<HItemListener> ls = new java.util.concurrent.CopyOnWriteArrayList<HItemListener>();
    private final Object lock = new Object();
    private final Set<HListElement> chosen = Collections.newSetFromMap(new IdentityHashMap<HListElement, Boolean>());
    private volatile HListElement[] items = new HListElement[0];
    private int current = ITEM_NOT_FOUND, scroll, orientation = ORIENT_TOP_TO_BOTTOM;
    private boolean multi, selecting;
    private Dimension icon = new Dimension(DEFAULT_ICON_WIDTH, DEFAULT_ICON_HEIGHT), label = new Dimension(DEFAULT_LABEL_WIDTH, DEFAULT_LABEL_HEIGHT);
    private HSound sound;
    public HListGroup() { super(look); }
    public HListGroup(HListElement[] e) { this(); setListContent(e); }
    public HListGroup(HListElement[] e, int x, int y, int w, int h) { this(e); setBounds(x, y, w, h); }
    public HListElement[] getListContent() { HListElement[] a = items; return a.length == 0 ? null : (HListElement[]) a.clone(); }
    public void setListContent(HListElement[] e) {
        synchronized (lock) {
            items = e == null ? new HListElement[0] : (HListElement[]) e.clone();
            chosen.clear();
            current = items.length == 0 ? ITEM_NOT_FOUND : 0;
            scroll = 0;
        }
        changed(LIST_CONTENT_CHANGE, null);
    }
    public void addItem(HListElement e, int at) { if (e != null) addItems(new HListElement[] {e}, at); }
    public void addItems(HListElement[] e, int at) {
        if (e == null || e.length == 0) return;
        synchronized (lock) {
            List<HListElement> l = new ArrayList<HListElement>(Arrays.asList(items));
            int i = at == ADD_INDEX_END ? l.size() : Math.max(0, Math.min(l.size(), at));
            l.addAll(i, Arrays.asList(e));
            items = l.toArray(new HListElement[l.size()]);
            if (current == ITEM_NOT_FOUND) current = 0;
            else if (i <= current) current += e.length;
        }
        changed(LIST_CONTENT_CHANGE, null);
    }
    public HListElement getItem(int i) { HListElement[] a = items; return i < 0 || i >= a.length ? null : a[i]; }
    public int getIndex(HListElement e) { HListElement[] a = items; for (int i = 0; i < a.length; i++) if (a[i] == e) return i; return ITEM_NOT_FOUND; }
    public int getNumItems() { return items.length; }
    public HListElement removeItem(int i) {
        HListElement gone;
        synchronized (lock) {
            if (i < 0 || i >= items.length) return null;
            List<HListElement> l = new ArrayList<HListElement>(Arrays.asList(items));
            gone = l.remove(i);
            items = l.toArray(new HListElement[l.size()]);
            chosen.remove(gone);
            if (items.length == 0) current = ITEM_NOT_FOUND;
            else if (current >= items.length || i < current) current = Math.max(0, current - 1);
            scroll = Math.max(0, Math.min(scroll, items.length - 1));
        }
        changed(LIST_CONTENT_CHANGE, null);
        return gone;
    }
    public void removeAllItems() { setListContent(null); }
    public int getCurrentIndex() { synchronized (lock) { return current; } }
    public HListElement getCurrentItem() { return getItem(getCurrentIndex()); }
    public boolean setCurrentItem(int i) {
        synchronized (lock) {
            if (i < 0 || i >= items.length) return false;
            current = i;
        }
        changed(ITEM_VALUE_CHANGE, null);
        return true;
    }
    public int[] getSelectionIndices() {
        synchronized (lock) {
            if (chosen.isEmpty()) return null;
            int[] o = new int[chosen.size()];
            int n = 0;
            for (int i = 0; i < items.length; i++) if (chosen.contains(items[i])) o[n++] = i;
            return n == o.length ? o : Arrays.copyOf(o, n);
        }
    }
    public HListElement[] getSelection() {
        synchronized (lock) {
            if (chosen.isEmpty()) return null;
            List<HListElement> o = new ArrayList<HListElement>();
            for (int i = 0; i < items.length; i++) if (chosen.contains(items[i])) o.add(items[i]);
            return o.toArray(new HListElement[o.size()]);
        }
    }
    public void clearSelection() { synchronized (lock) { chosen.clear(); } changed(LIST_SELECTION_CHANGE, null); }
    public int getNumSelected() { synchronized (lock) { return chosen.size(); } }
    public boolean getMultiSelection() { return multi; }
    public void setMultiSelection(boolean m) {
        synchronized (lock) {
            multi = m;
            if (!m && chosen.size() > 1) chosen.clear();
        }
        changed(LIST_MULTISELECTION_CHANGE, null);
    }
    public void setItemSelected(int i, boolean on) {
        synchronized (lock) {
            if (i < 0 || i >= items.length) return;
            if (on && !multi) chosen.clear();
            if (on) chosen.add(items[i]);
            else chosen.remove(items[i]);
        }
        changed(LIST_SELECTION_CHANGE, null);
    }
    public boolean isItemSelected(int i) { synchronized (lock) { return i >= 0 && i < items.length && chosen.contains(items[i]); } }
    public Dimension getIconSize() { return new Dimension(icon); }
    public void setIconSize(Dimension d) { icon = d == null ? new Dimension(DEFAULT_ICON_WIDTH, DEFAULT_ICON_HEIGHT) : new Dimension(d); changed(LIST_ICONSIZE_CHANGE, null); }
    public Dimension getLabelSize() { return new Dimension(label); }
    public void setLabelSize(Dimension d) { label = d == null ? new Dimension(DEFAULT_LABEL_WIDTH, DEFAULT_LABEL_HEIGHT) : new Dimension(d); changed(LIST_LABELSIZE_CHANGE, null); }
    public int getScrollPosition() { synchronized (lock) { return items.length == 0 ? ITEM_NOT_FOUND : scroll; } }
    public void setScrollPosition(int p) {
        synchronized (lock) {
            if (p < 0 || p >= items.length) return;
            scroll = p;
        }
        changed(LIST_SCROLLPOSITION_CHANGE, null);
    }
    public int getOrientation() { return orientation; }
    public void setOrientation(int o) { if (o >= ORIENT_LEFT_TO_RIGHT && o <= ORIENT_TOP_TO_BOTTOM) { orientation = o; changed(ORIENTATION_CHANGE, Integer.valueOf(o)); } }
    public boolean getSelectionMode() { return selecting; }
    public void setSelectionMode(boolean on) { selecting = on; setStateBit(ACTIONED_STATE_BIT, on); }
    public void addItemListener(HItemListener l) { if (l != null) ls.add(l); }
    public void removeItemListener(HItemListener l) { ls.remove(l); }
    public void setSelectionSound(HSound s) { sound = s; }
    public HSound getSelectionSound() { return sound; }
    public void processHFocusEvent(org.havi.ui.event.HFocusEvent e) {
        if (e.getID() == org.havi.ui.event.HFocusEvent.FOCUS_LOST && selecting) setSelectionMode(false);
        super.processHFocusEvent(e);
    }
    public void processHItemEvent(HItemEvent e) {
        if ((getInteractionState() & DISABLED_STATE_BIT) != 0) return;
        int n = items.length, at = getCurrentIndex();
        switch (e.getID()) {
            case HItemEvent.ITEM_START_CHANGE: setSelectionMode(true); return;
            case HItemEvent.ITEM_END_CHANGE: setSelectionMode(false); return;
            case HItemEvent.ITEM_TOGGLE_SELECTED:
                if (at < 0) return;
                setItemSelected(at, !isItemSelected(at));
                selected(new HItemEvent(this, e.getID(), getItem(at)));
                return;
            case HItemEvent.ITEM_SELECTED: if (at >= 0) { setItemSelected(at, true); selected(new HItemEvent(this, e.getID(), getItem(at))); } return;
            case HItemEvent.ITEM_CLEARED: if (at >= 0) { setItemSelected(at, false); selected(new HItemEvent(this, e.getID(), getItem(at))); } return;
            case HItemEvent.ITEM_SELECTION_CLEARED: clearSelection(); selected(e); return;
            case HItemEvent.ITEM_SET_CURRENT: if (e.getItem() instanceof HListElement) moved(getIndex((HListElement) e.getItem()), e.getID()); return;
            case HItemEvent.ITEM_SET_PREVIOUS: if (at <= 0) setSelectionMode(false); else moved(at - 1, e.getID()); return;
            case HItemEvent.ITEM_SET_NEXT: if (at < 0 || at >= n - 1) setSelectionMode(false); else moved(at + 1, e.getID()); return;
            case HItemEvent.SCROLL_MORE: setScrollPosition(getScrollPosition() + 1); return;
            case HItemEvent.SCROLL_LESS: setScrollPosition(getScrollPosition() - 1); return;
            case HItemEvent.SCROLL_PAGE_MORE: setScrollPosition(Math.min(n - 1, getScrollPosition() + page())); return;
            case HItemEvent.SCROLL_PAGE_LESS: setScrollPosition(Math.max(0, getScrollPosition() - page())); return;
            default: return;
        }
    }
    private int page() { HLook l = getLook(); return l instanceof HListGroupLook ? ((HListGroupLook) l).getNumVisible(this) : 1; }
    private void selected(HItemEvent e) { play(sound); for (HItemListener l : ls) l.selectionChanged(e); }
    private void moved(int i, int id) {
        if (!setCurrentItem(i)) return;
        // Keep the current element in sight.
        int p = page(), s = getScrollPosition();
        if (i < s) setScrollPosition(i);
        else if (i >= s + p) setScrollPosition(i - p + 1);
        HItemEvent e = new HItemEvent(this, id, getItem(i));
        for (HItemListener l : ls) l.currentItemChanged(e);
    }
}
