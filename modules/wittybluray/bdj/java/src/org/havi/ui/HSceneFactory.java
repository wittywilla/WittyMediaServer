package org.havi.ui;

import java.awt.Dimension;
import java.util.ArrayList;
import java.util.List;
import witty.bdj.Apps;

public class HSceneFactory {
    private static final HSceneFactory it = new HSceneFactory();
    private static final List<HScene> scenes = new ArrayList<HScene>();
    private static final class Scene extends HScene {}
    protected HSceneFactory() {}
    public static HSceneFactory getInstance() { witty.bdj.Keys.init(); return it; }
    static void remember(HScene s) { synchronized (scenes) { scenes.add(s); } }
    static void forget(HScene s) { synchronized (scenes) { scenes.remove(s); } }
    public HScene getBestScene(HSceneTemplate t) {
        HScene s = new Scene();
        if (t != null) s.setBounds(t.bounds());
        return s;
    }
    /** The calling application's own scene; every application has its own. */
    public HScene getDefaultHScene() {
        Apps.App a = Apps.owner();
        synchronized (scenes) {
            for (HScene s : scenes) if (s.owner() == a && !s.gone()) return s;
            return new Scene();
        }
    }
    public HScene getDefaultHScene(HScreen s) { return getDefaultHScene(); }
    public HScene getFullScreenScene(HGraphicsDevice d) { return new Scene(); }
    public HScene getFullScreenScene(HGraphicsDevice d, Dimension res) { return new Scene(); }
    public HSceneTemplate getBestSceneTemplate(HSceneTemplate t) { return t != null ? t : new HSceneTemplate(); }
    public HSceneTemplate resizeScene(HScene s, HSceneTemplate t) {
        if (s == null) throw new IllegalStateException();
        if (t != null) s.setBounds(t.bounds());
        return s.getSceneTemplate();
    }
    public void dispose(HScene s) { if (s != null) s.dispose(); }
}
