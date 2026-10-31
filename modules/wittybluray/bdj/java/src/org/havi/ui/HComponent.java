package org.havi.ui;

import java.awt.AlphaComposite;
import java.awt.Component;
import java.awt.Composite;
import java.awt.Graphics;
import java.awt.Graphics2D;
import java.awt.Image;
import java.awt.Point;
import java.awt.event.ActionEvent;
import java.awt.event.FocusEvent;
import java.awt.event.FocusListener;
import java.awt.event.KeyEvent;
import java.awt.image.BufferedImage;
import org.havi.ui.event.HActionEvent;
import org.havi.ui.event.HAdjustmentEvent;
import org.havi.ui.event.HFocusEvent;
import org.havi.ui.event.HItemEvent;
import org.havi.ui.event.HKeyEvent;
import org.havi.ui.event.HTextEvent;
import witty.bdj.Host;
import witty.bdj.Keys;
import witty.bdj.Plane;

public abstract class HComponent extends Component implements HMatteLayer, org.dvb.ui.TestOpacity {
    private HMatte matte;
    private boolean on = true;
    public HComponent() {}
    public HComponent(int x, int y, int w, int h) { setBounds(x, y, w, h); }
    public void setMatte(HMatte m) throws HMatteException { matte = m; Plane.invalidate(); }
    public HMatte getMatte() { return matte; }
    public boolean isOpaque() { return false; }
    public boolean isDoubleBuffered() { return false; }
    public void setEnabled(boolean e) { on = e; }
    public boolean isEnabled() { return on; }
    public void repaint(long t, int x, int y, int w, int h) { Plane.invalidate(); }
    public void requestFocus() { focus(this); }
    public boolean requestFocusInWindow() { focus(this); return true; }
    public boolean hasFocus() { return Keys.getFocus() == this; }
    public boolean isFocusOwner() { return hasFocus(); }
    public Image createImage(int w, int h) { return Plane.makeImage(w, h); }

    /** Runs on the event queue, where disc code may take as long as it likes without holding up the link. */
    static void later(final Runnable r) {
        java.awt.EventQueue.invokeLater(new Runnable() {
            public void run() {
                try {
                    r.run();
                } catch (Throwable t) {
                    Host.log("widget: " + t);
                }
            }
        });
    }

    /** Hands the focus over; components hear about it on the event queue, as with any other event. */
    static void focus(final Component to) {
        final Component from = Keys.getFocus();
        if (from == to) return;
        Keys.setFocus(to);
        later(new Runnable() {
            public void run() {
                if (from != null) tell(from, FocusEvent.FOCUS_LOST);
                if (to != null) tell(to, FocusEvent.FOCUS_GAINED);
            }
        });
    }
    private static void tell(Component c, int id) {
        HFocusEvent e = new HFocusEvent(c, id);
        if (c instanceof HNavigationInputPreferred) ((HNavigationInputPreferred) c).processHFocusEvent(e);
        FocusListener[] l = c.getFocusListeners();
        for (int i = 0; i < l.length; i++) {
            if (id == FocusEvent.FOCUS_GAINED) l[i].focusGained(e);
            else l[i].focusLost(e);
        }
    }

    /** The keys a focused widget acts on by itself: edit, adjust and select modes, the action key, then moves. */
    public static void navigate(Component c, int code, int id) {
        if (id != KeyEvent.KEY_PRESSED || !(c instanceof HNavigable)) return;
        if (c instanceof HKeyboardInputPreferred && entry(c, (HKeyboardInputPreferred) c, code)) return;
        if (c instanceof HAdjustmentInputPreferred && adjust((HAdjustmentInputPreferred) c, code)) return;
        if (c instanceof HSelectionInputPreferred && select((HSelectionInputPreferred) c, code)) return;
        if (code == KeyEvent.VK_ENTER && c instanceof HActionInputPreferred) {
            HActionInputPreferred a = (HActionInputPreferred) c;
            a.processHActionEvent(new HActionEvent(a, ActionEvent.ACTION_PERFORMED, a.getActionCommand()));
            return;
        }
        HNavigable n = (HNavigable) c;
        HNavigable next = n.getMove(code);
        if (next == n || !(next instanceof Component)) return;
        n.processHFocusEvent(new HFocusEvent(c, HFocusEvent.FOCUS_TRANSFER, code));
        focus((Component) next);
    }
    private static boolean entry(Component c, HKeyboardInputPreferred k, int code) {
        if (code == KeyEvent.VK_ENTER) {
            k.processHTextEvent(new HTextEvent(k, k.getEditMode() ? HTextEvent.TEXT_END_CHANGE : HTextEvent.TEXT_START_CHANGE));
            return true;
        }
        if (!k.getEditMode()) return false;
        int e;
        switch (code) {
            case KeyEvent.VK_LEFT: e = HTextEvent.CARET_PREV_CHAR; break;
            case KeyEvent.VK_RIGHT: e = HTextEvent.CARET_NEXT_CHAR; break;
            case KeyEvent.VK_UP: e = HTextEvent.CARET_PREV_LINE; break;
            case KeyEvent.VK_DOWN: e = HTextEvent.CARET_NEXT_LINE; break;
            case KeyEvent.VK_PAGE_UP: e = HTextEvent.CARET_PREV_PAGE; break;
            case KeyEvent.VK_PAGE_DOWN: e = HTextEvent.CARET_NEXT_PAGE; break;
            default:
                boolean text = (code >= KeyEvent.VK_0 && code <= KeyEvent.VK_9) || (code >= KeyEvent.VK_A && code <= KeyEvent.VK_Z) || code == KeyEvent.VK_SPACE;
                k.processHKeyEvent(new HKeyEvent(c, KeyEvent.KEY_PRESSED, java.lang.System.currentTimeMillis(), 0, code, text ? (char) code : KeyEvent.CHAR_UNDEFINED));
                return true;
        }
        k.processHTextEvent(new HTextEvent(k, e));
        return true;
    }
    private static boolean adjust(HAdjustmentInputPreferred a, int code) {
        if (code == KeyEvent.VK_ENTER) {
            a.processHAdjustmentEvent(new HAdjustmentEvent(a, a.getAdjustMode() ? HAdjustmentEvent.ADJUST_END_CHANGE : HAdjustmentEvent.ADJUST_START_CHANGE));
            return true;
        }
        if (!a.getAdjustMode()) return false;
        int e;
        switch (code) {
            case KeyEvent.VK_LEFT: case KeyEvent.VK_DOWN: e = HAdjustmentEvent.ADJUST_LESS; break;
            case KeyEvent.VK_RIGHT: case KeyEvent.VK_UP: e = HAdjustmentEvent.ADJUST_MORE; break;
            case KeyEvent.VK_PAGE_DOWN: e = HAdjustmentEvent.ADJUST_PAGE_LESS; break;
            case KeyEvent.VK_PAGE_UP: e = HAdjustmentEvent.ADJUST_PAGE_MORE; break;
            default: return false;
        }
        a.processHAdjustmentEvent(new HAdjustmentEvent(a, e));
        return true;
    }
    private static boolean select(HSelectionInputPreferred s, int code) {
        boolean on = s.getSelectionMode();
        if (code == KeyEvent.VK_ENTER) {
            s.processHItemEvent(new HItemEvent(s, on ? HItemEvent.ITEM_TOGGLE_SELECTED : HItemEvent.ITEM_START_CHANGE, null));
            return true;
        }
        if (!on) return false;
        int e;
        switch (code) {
            case KeyEvent.VK_UP: case KeyEvent.VK_LEFT: e = HItemEvent.ITEM_SET_PREVIOUS; break;
            case KeyEvent.VK_DOWN: case KeyEvent.VK_RIGHT: e = HItemEvent.ITEM_SET_NEXT; break;
            case KeyEvent.VK_PAGE_UP: e = HItemEvent.SCROLL_PAGE_LESS; break;
            case KeyEvent.VK_PAGE_DOWN: e = HItemEvent.SCROLL_PAGE_MORE; break;
            case KeyEvent.VK_ESCAPE: e = HItemEvent.ITEM_END_CHANGE; break;
            default: return false;
        }
        s.processHItemEvent(new HItemEvent(s, e, null));
        return true;
    }

    interface Drawing { void draw(Graphics g); }
    /** Draws through a matte, by way of a buffer so that overlapping content fades as one. */
    static void through(HMatte m, Graphics g, int w, int h, Drawing d) {
        if (m == null || !(g instanceof Graphics2D) || w <= 0 || h <= 0) {
            d.draw(g);
            return;
        }
        float alpha = 1;
        Image pic = null;
        Point at = null;
        if (m instanceof HFlatMatte) alpha = ((HFlatMatte) m).getMatteData();
        else if (m instanceof HFlatEffectMatte) alpha = ((HFlatEffectMatte) m).now();
        else if (m instanceof HImageMatte) { pic = ((HImageMatte) m).getMatteData(); at = ((HImageMatte) m).getOffset(); }
        else if (m instanceof HImageEffectMatte) { HImageEffectMatte e = (HImageEffectMatte) m; int p = e.getPosition(); Image[] a = e.getMatteData(); pic = a == null || p >= a.length ? null : a[p]; at = e.getOffset(p); }
        BufferedImage b = Plane.makeImage(w, h);
        Graphics2D s = b.createGraphics();
        try {
            d.draw(new org.dvb.ui.DVBGraphicsImpl(s));
            if (pic != null) {
                s.setComposite(AlphaComposite.DstIn);
                s.drawImage(pic, at == null ? 0 : at.x, at == null ? 0 : at.y, null);
            }
        } finally {
            s.dispose();
        }
        Graphics2D o = (Graphics2D) g;
        Composite old = o.getComposite();
        o.setComposite(AlphaComposite.SrcOver.derive(Math.max(0f, Math.min(1f, alpha))));
        o.drawImage(b, 0, 0, null);
        o.setComposite(old);
    }
}
