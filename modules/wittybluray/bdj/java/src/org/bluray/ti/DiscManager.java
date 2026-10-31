package org.bluray.ti;

import java.util.List;

/** One disc for the whole life of the player, so its status never changes. */
public final class DiscManager {
    private static final DiscManager it = new DiscManager();
    private final Disc disc = new DiscImpl();
    private final List<DiscStatusListener> ls = new java.util.concurrent.CopyOnWriteArrayList<DiscStatusListener>();
    private DiscManager() {}
    public static DiscManager getDiscManager() { return it; }
    public Disc getCurrentDisc() { return disc; }
    public void selectDisc(org.bluray.net.BDLocator l) {}
    public void expectNextDisc(String[] ids) {}
    public void addDiscStatusEventListener(DiscStatusListener l) { if (l != null) ls.add(l); }
    public void removeDiscStatusEventListener(DiscStatusListener l) { ls.remove(l); }
}
