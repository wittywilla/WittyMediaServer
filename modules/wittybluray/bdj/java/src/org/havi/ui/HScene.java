package org.havi.ui;

import java.awt.Component;
import java.awt.Container;
import java.awt.Graphics;
import java.awt.Graphics2D;
import java.awt.Image;
import java.awt.Point;
import java.awt.Rectangle;
import java.awt.event.ActionEvent;
import java.awt.event.KeyEvent;
import java.awt.event.WindowListener;
import org.havi.ui.event.HActionEvent;
import witty.bdj.Apps;
import witty.bdj.Host;
import witty.bdj.Keys;
import witty.bdj.Plane;

/** The top of an application's on-screen tree; the plane paints it for as long as its application lives. */
public class HScene extends Container implements HComponentOrdering, Plane.Layer, org.dvb.ui.TestOpacity {
    public static final int IMAGE_NONE = 0, IMAGE_STRETCH = 1, IMAGE_CENTER = 2, IMAGE_TILE = 3;
    public static final int NO_BACKGROUND_FILL = 0, BACKGROUND_FILL = 1;
    private int bgMode = NO_BACKGROUND_FILL, renderMode = IMAGE_NONE;
    private Image bgImage;
    private volatile boolean active;
    private final java.util.List<WindowListener> win = new java.util.concurrent.CopyOnWriteArrayList<WindowListener>();
    private final java.util.Map<Integer, HActionable> shortcuts = new java.util.concurrent.ConcurrentHashMap<Integer, HActionable>();
    private volatile boolean shortcutsOn = true, disposed;
    private final Keys.Sink sink;
    private final Apps.App owner = Apps.owner();
    private final ThreadGroup ownerGroup = owner == null ? null : owner.group;
    private long goneSince;
    private boolean reaping;
    private org.dvb.event.UserEventRepository keyEvents;
    // Scenes need a parent with no peer: AWT passes event masks up to the first such container.
    private static final Container ROOT = new Container();

    protected HScene() {
        setBounds(0, 0, Host.width, Host.height);
        setLayout(null);
        setVisible(false);
        addNotify();
        synchronized (ROOT) { ROOT.setLayout(null); ROOT.add(this); }
        Plane.add(this);
        sink = new Keys.Sink() {
            public boolean key(int code, int id, long when) {
                if (gone() || !isVisible() || !active) return false;
                if (id == KeyEvent.KEY_PRESSED && shortcutsOn) {
                    HActionable a = shortcuts.get(code);
                    if (a != null) {
                        a.processHActionEvent(new HActionEvent(a, ActionEvent.ACTION_PERFORMED, a.getActionCommand()));
                        return true;
                    }
                }
                return Keys.deliver(HScene.this, code, id, when);
            }
        };
        Keys.addSink(sink);
        active = true;
        HSceneFactory.remember(this);
    }

    final Apps.App owner() { return owner; }
    /** Disposed, or its application has ended or been started over: not on the screen again, whatever the application does now. */
    final boolean gone() { return disposed || (owner != null && (owner.ended() || owner.group != ownerGroup)); }
    public final boolean visibleLayer() {
        if (!gone()) return isVisible();
        reap();
        return false;
    }
    /**
     * The plane asks about every scene several times a second, which makes it the one place to notice an application
     * however it ended. destroyXlet gets a moment to do its own tidying, and disposing runs on a thread of its own, since
     * it can run into disc code.
     */
    private void reap() {
        if (disposed || reaping) return;
        long now = java.lang.System.currentTimeMillis();
        if (goneSince == 0) goneSince = now;
        if (now - goneSince < 2000) return;
        reaping = true;
        Thread t = new Thread(new Runnable() {
            public void run() {
                try {
                    dispose();
                } catch (Throwable e) {
                    Host.log("scene: " + e);
                }
            }
        }, "scene-end");
        t.setDaemon(true);
        t.start();
    }
    public final void paintLayer(Graphics2D g) {
        g.translate(getX(), getY());
        g.clipRect(0, 0, getWidth(), getHeight());
        java.awt.image.BufferedImage s = surface;
        if (s == null) {
            paint(new org.dvb.ui.DVBGraphicsImpl(g));
            return;
        }
        // A scene drawn on directly works like a real graphics plane: what is drawn stays,
        // and components paint into it only when they asked to, not on every frame shown.
        if (treeDirty) {
            treeDirty = false;
            Graphics2D t = s.createGraphics();
            try {
                paint(new org.dvb.ui.DVBGraphicsImpl(t));
            } finally {
                t.dispose();
            }
        }
        g.drawImage(s, 0, 0, null);
    }
    private volatile boolean treeDirty = true;
    public void paint(Graphics g) {
        if (bgMode == BACKGROUND_FILL && getBackground() != null) {
            g.setColor(getBackground());
            g.fillRect(0, 0, getWidth(), getHeight());
        }
        if (bgImage != null && renderMode != IMAGE_NONE) {
            if (renderMode == IMAGE_STRETCH) g.drawImage(bgImage, 0, 0, getWidth(), getHeight(), this);
            else if (renderMode == IMAGE_CENTER) g.drawImage(bgImage, (getWidth() - bgImage.getWidth(this)) / 2, (getHeight() - bgImage.getHeight(this)) / 2, this);
            else for (int y = 0; y < getHeight(); y += Math.max(1, bgImage.getHeight(this))) for (int x = 0; x < getWidth(); x += Math.max(1, bgImage.getWidth(this))) g.drawImage(bgImage, x, y, this);
        }
        super.paint(g);
    }
    public void repaint(long tm, int x, int y, int w, int h) {
        treeDirty = true;
        Plane.invalidate();
    }
    public void update(Graphics g) { paint(g); }
    public Graphics getGraphics() {
        // Direct drawing: Toolkit.sync says when the picture is ready to show.
        Plane.invalidate();
        return new org.dvb.ui.DVBGraphicsImpl(surface().createGraphics());
    }
    private volatile java.awt.image.BufferedImage surface;
    private synchronized java.awt.image.BufferedImage surface() {
        if (surface == null) surface = Plane.makeImage(getWidth(), getHeight());
        return surface;
    }
    public Image createImage(int w, int h) { return Plane.makeImage(w, h); }
    public java.awt.image.VolatileImage createVolatileImage(int w, int h) { return null; }
    public boolean isShowing() { return isVisible(); }
    public void setActive(boolean a) { active = a; }
    public boolean isActive() { return active; }
    public void show() { super.show(); Plane.invalidate(); }
    public void hide() { super.hide(); Plane.invalidate(); }
    /** Keeps the focus a component in the scene has, and takes it otherwise. */
    public void requestFocus() {
        for (Component c = Keys.getFocus(); c != null; c = c.getParent()) if (c == this) return;
        HComponent.focus(this);
    }
    public boolean requestFocusInWindow() { requestFocus(); return true; }
    public boolean hasFocus() { return Keys.getFocus() == this; }
    public boolean isFocusOwner() { return hasFocus(); }
    public void dispose() {
        boolean again = disposed;
        disposed = true;
        Plane.remove(this);
        Keys.removeSink(sink);
        HSceneFactory.forget(this);
        if (again) return;
        for (Component c = Keys.getFocus(); c != null; c = c.getParent()) {
            if (c == this) { Keys.setFocus(null); break; }
        }
        setVisible(false);
        synchronized (ROOT) { ROOT.remove(this); }
        removeAll();
        shortcuts.clear();
    }
    protected void addImpl(Component c, Object k, int i) { super.addImpl(c, k, i); Plane.invalidate(); }
    public void remove(int i) { super.remove(i); Plane.invalidate(); }
    public void remove(Component c) { super.remove(c); Plane.invalidate(); }
    public void setBackgroundMode(int m) { bgMode = m; Plane.invalidate(); }
    public int getBackgroundMode() { return bgMode; }
    public void setBackgroundImage(Image i) { bgImage = i; Plane.invalidate(); }
    public Image getBackgroundImage() { return bgImage; }
    public boolean setRenderMode(int m) {
        if (m < IMAGE_NONE || m > IMAGE_TILE) return false;
        renderMode = m;
        Plane.invalidate();
        return true;
    }
    public int getRenderMode() { return renderMode; }
    public boolean isOpaque() { return bgMode == BACKGROUND_FILL; }
    public boolean isDoubleBuffered() { return false; }
    public void addWindowListener(WindowListener l) { if (l != null) win.add(l); }
    public void removeWindowListener(WindowListener l) { win.remove(l); }
    public HSceneTemplate getSceneTemplate() {
        HSceneTemplate t = new HSceneTemplate();
        t.setPreference(HSceneTemplate.SCENE_PIXEL_DIMENSION, getSize(), HSceneTemplate.REQUIRED);
        t.setPreference(HSceneTemplate.SCENE_PIXEL_LOCATION, new Point(getX(), getY()), HSceneTemplate.REQUIRED);
        return t;
    }
    public Rectangle getPixelCoordinatesHScreenRectangle(HScreenRectangle r) {
        return new Rectangle(Math.round(r.x * Host.width) - getX(), Math.round(r.y * Host.height) - getY(), Math.round(r.width * Host.width), Math.round(r.height * Host.height));
    }
    public boolean addShortcut(int code, HActionable a) {
        if (a == null) return false;
        shortcuts.put(code, a);
        return true;
    }
    public void removeShortcut(int code) { shortcuts.remove(code); }
    public HActionable getShortcutComponent(int code) { return shortcuts.get(code); }
    public void enableShortcuts(boolean e) { shortcutsOn = e; }
    public boolean isEnableShortcuts() { return shortcutsOn; }
    public int getShortcutKeycode(HActionable a) {
        for (java.util.Map.Entry<Integer, HActionable> e : shortcuts.entrySet()) if (e.getValue() == a) return e.getKey();
        return java.awt.event.KeyEvent.VK_UNDEFINED;
    }
    public int[] getAllShortcutKeycodes() {
        Integer[] all = shortcuts.keySet().toArray(new Integer[0]);
        int[] k = new int[all.length];
        for (int i = 0; i < k.length; i++) k[i] = all[i];
        return k;
    }
    public void setKeyEvents(org.dvb.event.UserEventRepository r) { keyEvents = r; }
    public org.dvb.event.UserEventRepository getKeyEvents() { return keyEvents; }
    public Component addBefore(Component c, Component b) { return HContainer.place(this, c, b, 0); }
    public Component addAfter(Component c, Component b) { return HContainer.place(this, c, b, 1); }
    public boolean popToFront(Component c) { return HContainer.shift(this, c, 0); }
    public boolean pushToBack(Component c) { return HContainer.shift(this, c, Integer.MAX_VALUE); }
    public boolean pop(Component c) { return HContainer.step(this, c, -1); }
    public boolean push(Component c) { return HContainer.step(this, c, 1); }
    public boolean popInFrontOf(Component m, Component n) { return HContainer.index(this, m) >= 0 && HContainer.place(this, m, n, 0) != null; }
    public boolean pushBehind(Component m, Component n) { return HContainer.index(this, m) >= 0 && HContainer.place(this, m, n, 1) != null; }
}
