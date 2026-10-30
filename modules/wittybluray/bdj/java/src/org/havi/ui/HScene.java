package org.havi.ui;

import java.awt.AlphaComposite;
import java.awt.Color;
import java.awt.Component;
import java.awt.Container;
import java.awt.Dimension;
import java.awt.Graphics;
import java.awt.Graphics2D;
import java.awt.Image;
import java.awt.Point;
import java.awt.event.KeyEvent;
import java.awt.event.WindowListener;
import java.awt.image.ImageObserver;
import witty.bdj.Host;
import witty.bdj.Keys;
import witty.bdj.Plane;

/** The top of an application's on-screen tree; the plane paints it. */
public class HScene extends Container implements HComponentOrdering, Plane.Layer, org.dvb.ui.TestOpacity {
    public static final int IMAGE_NONE = 0, IMAGE_STRETCH = 1, IMAGE_CENTER = 2, IMAGE_TILE = 3;
    public static final int NO_BACKGROUND_FILL = 0, BACKGROUND_FILL = 1;
    private int bgMode = NO_BACKGROUND_FILL, renderMode = IMAGE_NONE;
    private Image bgImage;
    private boolean active, shown = true;
    private final java.util.List<WindowListener> win = new java.util.ArrayList<WindowListener>();
    private final Keys.Sink sink;
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
                if (!isVisible() || !active) return false;
                return Keys.deliver(HScene.this, code, id, when);
            }
        };
        Keys.addSink(sink);
        active = true;
    }

    public boolean visibleLayer() { return isVisible(); }
    public void paintLayer(Graphics2D g) {
        g.translate(getX(), getY());
        g.clipRect(0, 0, getWidth(), getHeight());
        paint(new org.dvb.ui.DVBGraphicsImpl(g));
    }
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
    public void repaint(long tm, int x, int y, int w, int h) { Plane.invalidate(); }
    public void update(Graphics g) { paint(g); }
    public Graphics getGraphics() {
        // Direct drawing: a picture the scene shows on its next paint, as discs that double-buffer by hand expect.
        Plane.invalidate();
        return new org.dvb.ui.DVBGraphicsImpl(surface().createGraphics());
    }
    private java.awt.image.BufferedImage surface;
    private java.awt.image.BufferedImage surface() {
        if (surface == null) {
            surface = Plane.makeImage(getWidth(), getHeight());
            bgImage = surface;
            renderMode = IMAGE_STRETCH;
        }
        return surface;
    }
    public Image createImage(int w, int h) { return Plane.makeImage(w, h); }
    public java.awt.image.VolatileImage createVolatileImage(int w, int h) { return null; }
    public boolean isShowing() { return isVisible(); }
    public void setActive(boolean a) { active = a; }
    public boolean isActive() { return active; }
    public void show() { super.show(); Plane.invalidate(); }
    public void hide() { super.hide(); Plane.invalidate(); }
    public void dispose() {
        setVisible(false);
        Plane.remove(this);
        Keys.removeSink(sink);
        HSceneFactory.forget(this);
        synchronized (ROOT) { ROOT.remove(this); }
        removeAll();
    }
    public boolean setBackgroundMode(int m) { bgMode = m; Plane.invalidate(); return true; }
    public int getBackgroundMode() { return bgMode; }
    public void setBackgroundImage(Image i) { bgImage = i; Plane.invalidate(); }
    public Image getBackgroundImage() { return bgImage; }
    public boolean setRenderMode(int m) { renderMode = m; Plane.invalidate(); return true; }
    public int getRenderMode() { return renderMode; }
    public boolean isOpaque() { return bgMode == BACKGROUND_FILL; }
    public boolean isDoubleBuffered() { return false; }
    public void addWindowListener(WindowListener l) { win.add(l); }
    public void removeWindowListener(WindowListener l) { win.remove(l); }
    public HSceneTemplate getSceneTemplate() { return new HSceneTemplate(); }
    public void setShortcut(int code, Object o) {}
    public boolean addShortcut(int code, HActionable a) { return true; }
    public void removeShortcut(int code) {}
    public HActionable getShortcutComponent(int code) { return null; }
    public void enableShortcuts(boolean e) {}
    public boolean isEnableShortcuts() { return true; }
    public int getShortcutKeycode(HActionable a) { return 0; }
    public int[] getAllShortcutKeycodes() { return new int[0]; }
    public void setKeyEvents(org.dvb.event.UserEventRepository r) { keyEvents = r; }
    public org.dvb.event.UserEventRepository getKeyEvents() { return keyEvents; }
    public Component addBefore(Component c, Component b) { return order(c, b, 0); }
    public Component addAfter(Component c, Component b) { return order(c, b, 1); }
    private Component order(Component c, Component b, int d) {
        Component[] all = getComponents();
        for (int i = 0; i < all.length; i++) if (all[i] == b) { remove(c); add(c, Math.min(i + d, getComponentCount())); return c; }
        return null;
    }
    public boolean popToFront(Component c) { remove(c); add(c, 0); return true; }
    public boolean pushToBack(Component c) { remove(c); add(c); return true; }
    public boolean pop(Component c) { return popToFront(c); }
    public boolean push(Component c) { return pushToBack(c); }
    public boolean popInFrontOf(Component m, Component n) { return addBefore(m, n) != null; }
    public boolean pushBehind(Component m, Component n) { return addAfter(m, n) != null; }
}
