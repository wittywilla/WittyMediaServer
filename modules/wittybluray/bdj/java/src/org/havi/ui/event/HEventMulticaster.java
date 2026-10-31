package org.havi.ui.event;

import java.awt.event.ActionEvent;
import java.awt.event.FocusEvent;
import java.awt.event.KeyEvent;
import java.awt.event.WindowEvent;
import java.awt.event.WindowListener;
import java.util.EventListener;

/** A pair of listeners that looks like one; chains of these are how widgets keep lists of listeners. */
public class HEventMulticaster implements HBackgroundImageListener, HScreenConfigurationListener, HScreenLocationModifiedListener, WindowListener,
        HActionListener, HAdjustmentListener, HFocusListener, HItemListener, HTextListener, HKeyListener {
    protected final EventListener a, b;
    protected HEventMulticaster(EventListener a, EventListener b) { this.a = a; this.b = b; }
    protected EventListener remove(EventListener old) {
        if (old == a) return b;
        if (old == b) return a;
        EventListener x = removeInternal(a, old), y = removeInternal(b, old);
        return x == a && y == b ? this : addInternal(x, y);
    }
    protected static EventListener addInternal(EventListener a, EventListener b) { return a == null ? b : b == null ? a : new HEventMulticaster(a, b); }
    protected static EventListener removeInternal(EventListener l, EventListener old) {
        if (l == old || l == null) return null;
        return l instanceof HEventMulticaster ? ((HEventMulticaster) l).remove(old) : l;
    }
    public static HBackgroundImageListener add(HBackgroundImageListener a, HBackgroundImageListener b) { return (HBackgroundImageListener) addInternal(a, b); }
    public static HBackgroundImageListener remove(HBackgroundImageListener l, HBackgroundImageListener o) { return (HBackgroundImageListener) removeInternal(l, o); }
    public static HScreenConfigurationListener add(HScreenConfigurationListener a, HScreenConfigurationListener b) { return (HScreenConfigurationListener) addInternal(a, b); }
    public static HScreenConfigurationListener remove(HScreenConfigurationListener l, HScreenConfigurationListener o) { return (HScreenConfigurationListener) removeInternal(l, o); }
    public static HScreenLocationModifiedListener add(HScreenLocationModifiedListener a, HScreenLocationModifiedListener b) { return (HScreenLocationModifiedListener) addInternal(a, b); }
    public static HScreenLocationModifiedListener remove(HScreenLocationModifiedListener l, HScreenLocationModifiedListener o) { return (HScreenLocationModifiedListener) removeInternal(l, o); }
    public static WindowListener add(WindowListener a, WindowListener b) { return (WindowListener) addInternal(a, b); }
    public static WindowListener remove(WindowListener l, WindowListener o) { return (WindowListener) removeInternal(l, o); }
    public static HActionListener add(HActionListener a, HActionListener b) { return (HActionListener) addInternal(a, b); }
    public static HActionListener remove(HActionListener l, HActionListener o) { return (HActionListener) removeInternal(l, o); }
    public static HAdjustmentListener add(HAdjustmentListener a, HAdjustmentListener b) { return (HAdjustmentListener) addInternal(a, b); }
    public static HAdjustmentListener remove(HAdjustmentListener l, HAdjustmentListener o) { return (HAdjustmentListener) removeInternal(l, o); }
    public static HFocusListener add(HFocusListener a, HFocusListener b) { return (HFocusListener) addInternal(a, b); }
    public static HFocusListener remove(HFocusListener l, HFocusListener o) { return (HFocusListener) removeInternal(l, o); }
    public static HItemListener add(HItemListener a, HItemListener b) { return (HItemListener) addInternal(a, b); }
    public static HItemListener remove(HItemListener l, HItemListener o) { return (HItemListener) removeInternal(l, o); }
    public static HTextListener add(HTextListener a, HTextListener b) { return (HTextListener) addInternal(a, b); }
    public static HTextListener remove(HTextListener l, HTextListener o) { return (HTextListener) removeInternal(l, o); }
    public static HKeyListener add(HKeyListener a, HKeyListener b) { return (HKeyListener) addInternal(a, b); }
    public static HKeyListener remove(HKeyListener l, HKeyListener o) { return (HKeyListener) removeInternal(l, o); }
    public void imageLoaded(HBackgroundImageEvent e) { if (a instanceof HBackgroundImageListener) ((HBackgroundImageListener) a).imageLoaded(e); if (b instanceof HBackgroundImageListener) ((HBackgroundImageListener) b).imageLoaded(e); }
    public void imageLoadFailed(HBackgroundImageEvent e) { if (a instanceof HBackgroundImageListener) ((HBackgroundImageListener) a).imageLoadFailed(e); if (b instanceof HBackgroundImageListener) ((HBackgroundImageListener) b).imageLoadFailed(e); }
    public void report(HScreenConfigurationEvent e) { if (a instanceof HScreenConfigurationListener) ((HScreenConfigurationListener) a).report(e); if (b instanceof HScreenConfigurationListener) ((HScreenConfigurationListener) b).report(e); }
    public void report(HScreenLocationModifiedEvent e) { if (a instanceof HScreenLocationModifiedListener) ((HScreenLocationModifiedListener) a).report(e); if (b instanceof HScreenLocationModifiedListener) ((HScreenLocationModifiedListener) b).report(e); }
    public void windowOpened(WindowEvent e) { if (a instanceof WindowListener) ((WindowListener) a).windowOpened(e); if (b instanceof WindowListener) ((WindowListener) b).windowOpened(e); }
    public void windowClosing(WindowEvent e) { if (a instanceof WindowListener) ((WindowListener) a).windowClosing(e); if (b instanceof WindowListener) ((WindowListener) b).windowClosing(e); }
    public void windowClosed(WindowEvent e) { if (a instanceof WindowListener) ((WindowListener) a).windowClosed(e); if (b instanceof WindowListener) ((WindowListener) b).windowClosed(e); }
    public void windowIconified(WindowEvent e) { if (a instanceof WindowListener) ((WindowListener) a).windowIconified(e); if (b instanceof WindowListener) ((WindowListener) b).windowIconified(e); }
    public void windowDeiconified(WindowEvent e) { if (a instanceof WindowListener) ((WindowListener) a).windowDeiconified(e); if (b instanceof WindowListener) ((WindowListener) b).windowDeiconified(e); }
    public void windowActivated(WindowEvent e) { if (a instanceof WindowListener) ((WindowListener) a).windowActivated(e); if (b instanceof WindowListener) ((WindowListener) b).windowActivated(e); }
    public void windowDeactivated(WindowEvent e) { if (a instanceof WindowListener) ((WindowListener) a).windowDeactivated(e); if (b instanceof WindowListener) ((WindowListener) b).windowDeactivated(e); }
    public void actionPerformed(ActionEvent e) { if (a instanceof HActionListener) ((HActionListener) a).actionPerformed(e); if (b instanceof HActionListener) ((HActionListener) b).actionPerformed(e); }
    public void valueChanged(HAdjustmentEvent e) { if (a instanceof HAdjustmentListener) ((HAdjustmentListener) a).valueChanged(e); if (b instanceof HAdjustmentListener) ((HAdjustmentListener) b).valueChanged(e); }
    public void focusGained(FocusEvent e) { if (a instanceof HFocusListener) ((HFocusListener) a).focusGained(e); if (b instanceof HFocusListener) ((HFocusListener) b).focusGained(e); }
    public void focusLost(FocusEvent e) { if (a instanceof HFocusListener) ((HFocusListener) a).focusLost(e); if (b instanceof HFocusListener) ((HFocusListener) b).focusLost(e); }
    public void selectionChanged(HItemEvent e) { if (a instanceof HItemListener) ((HItemListener) a).selectionChanged(e); if (b instanceof HItemListener) ((HItemListener) b).selectionChanged(e); }
    public void currentItemChanged(HItemEvent e) { if (a instanceof HItemListener) ((HItemListener) a).currentItemChanged(e); if (b instanceof HItemListener) ((HItemListener) b).currentItemChanged(e); }
    public void textChanged(HTextEvent e) { if (a instanceof HTextListener) ((HTextListener) a).textChanged(e); if (b instanceof HTextListener) ((HTextListener) b).textChanged(e); }
    public void caretMoved(HTextEvent e) { if (a instanceof HTextListener) ((HTextListener) a).caretMoved(e); if (b instanceof HTextListener) ((HTextListener) b).caretMoved(e); }
    public void keyTyped(KeyEvent e) { if (a instanceof HKeyListener) ((HKeyListener) a).keyTyped(e); if (b instanceof HKeyListener) ((HKeyListener) b).keyTyped(e); }
    public void keyPressed(KeyEvent e) { if (a instanceof HKeyListener) ((HKeyListener) a).keyPressed(e); if (b instanceof HKeyListener) ((HKeyListener) b).keyPressed(e); }
    public void keyReleased(KeyEvent e) { if (a instanceof HKeyListener) ((HKeyListener) a).keyReleased(e); if (b instanceof HKeyListener) ((HKeyListener) b).keyReleased(e); }
}
