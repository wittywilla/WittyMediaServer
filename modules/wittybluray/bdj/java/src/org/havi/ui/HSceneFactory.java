package org.havi.ui;

import java.awt.Dimension;
import java.util.ArrayList;
import java.util.List;

public class HSceneFactory {
    private static final HSceneFactory it = new HSceneFactory();
    private static final List<HScene> scenes = new ArrayList<HScene>();
    private static final class Scene extends HScene {}
    protected HSceneFactory() {}
    public static HSceneFactory getInstance() { witty.bdj.Keys.init(); return it; }
    static synchronized void forget(HScene s) { scenes.remove(s); }
    public synchronized HScene getBestScene(HSceneTemplate t) { return make(); }
    public synchronized HScene getDefaultHScene() { return scenes.isEmpty() ? make() : scenes.get(0); }
    public synchronized HScene getDefaultHScene(HScreen s) { return getDefaultHScene(); }
    public synchronized HScene getFullScreenScene(HGraphicsDevice d) { return make(); }
    public synchronized HScene getFullScreenScene(HGraphicsDevice d, Dimension res) { return make(); }
    public HSceneTemplate getBestSceneTemplate(HSceneTemplate t) { return t; }
    public HSceneTemplate resizeScene(HScene s, HSceneTemplate t) { return t; }
    public void dispose(HScene s) { s.dispose(); }
    private HScene make() {
        HScene s = new Scene();
        scenes.add(s);
        return s;
    }
}
