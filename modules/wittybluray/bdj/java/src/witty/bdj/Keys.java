package witty.bdj;

import java.awt.Component;
import java.awt.Container;
import java.awt.event.KeyEvent;
import java.awt.event.KeyListener;
import java.util.ArrayList;
import java.util.List;

/** Delivers the viewer's keys to the applications: first to whoever reserved them, then to the scene in focus. */
public final class Keys {
    private Keys() {}

    public interface Sink {
        /** true when the key was taken. */
        boolean key(int code, int id, long when);
    }

    private static final List<Sink> sinks = new java.util.concurrent.CopyOnWriteArrayList<Sink>();
    public static void addSink(Sink s) { sinks.add(s); }
    public static void addFirstSink(Sink s) { sinks.add(0, s); }
    public static void removeSink(Sink s) { sinks.remove(s); }

    static {
        Host.listen(new Host.Listener() {
            public void onMessage(String[] w) {
                if (!w[0].equals("key")) return;
                int code = Integer.parseInt(w[1]);
                long when = java.lang.System.currentTimeMillis();
                press(code, KeyEvent.KEY_PRESSED, when);
                press(code, KeyEvent.KEY_RELEASED, when + 1);
            }
        });
    }
    /** Makes sure the class is loaded, so the listener above exists. */
    public static void init() {}

    private static void press(final int code, final int id, final long when) {
        java.awt.EventQueue.invokeLater(new Runnable() {
            public void run() {
                for (Sink s : sinks) {
                    try {
                        if (s.key(code, id, when)) return;
                    } catch (Throwable t) {
                        Host.log("key: " + t);
                    }
                }
            }
        });
    }

    /** Sends a key event down a scene: its focused component, its listeners. */
    public static boolean deliver(Component root, int code, int id, long when) {
        KeyEvent e = new KeyEvent(root, id, when, 0, code, KeyEvent.CHAR_UNDEFINED);
        Component f = focused(root);
        boolean any = false;
        for (Component c = f; c != null; c = c.getParent()) {
            KeyListener[] l = c.getKeyListeners();
            for (int i = 0; i < l.length; i++) {
                any = true;
                if (id == KeyEvent.KEY_PRESSED) l[i].keyPressed(e);
                else l[i].keyReleased(e);
            }
            if (c instanceof org.havi.ui.HNavigable) org.havi.ui.HComponent.navigate(c, code, id);
            if (e.isConsumed() || c == root) break;
        }
        return any;
    }

    private static Component focusOwner;
    public static void setFocus(Component c) { focusOwner = c; }
    public static Component getFocus() { return focusOwner; }

    private static Component focused(Component root) {
        if (focusOwner != null && isInside(focusOwner, root)) return focusOwner;
        return root;
    }

    private static boolean isInside(Component c, Component root) {
        for (Component p = c; p != null; p = p.getParent()) if (p == root) return true;
        return false;
    }
}
