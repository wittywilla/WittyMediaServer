package org.bluray.ti;

public final class DiscManager {
    private static final DiscManager it = new DiscManager();
    private final Disc disc = new Disc();
    private DiscManager() {}
    public static DiscManager getDiscManager() { return it; }
    public Disc getCurrentDisc() { return disc; }
    public void selectDisc(org.bluray.net.BDLocator l) {}
    public void addDiscStatusListener(Object l) {}
    public void removeDiscStatusListener(Object l) {}
}
