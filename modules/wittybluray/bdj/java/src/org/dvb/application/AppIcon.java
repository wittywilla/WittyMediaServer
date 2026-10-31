package org.dvb.application;
public class AppIcon {
    private final javax.tv.locator.Locator loc;
    private final int flags;
    public AppIcon(javax.tv.locator.Locator l, int flags) { loc = l; this.flags = flags; }
    public javax.tv.locator.Locator getLocator() { return loc; }
    public int getIconFlags() { return flags; }
}
