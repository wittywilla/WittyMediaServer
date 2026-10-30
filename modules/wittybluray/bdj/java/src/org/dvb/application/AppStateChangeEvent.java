package org.dvb.application;
public class AppStateChangeEvent extends java.util.EventObject {
    private final AppID id;
    private final int from, to;
    private final boolean failed;
    public AppStateChangeEvent(AppID a, int f, int t, Object s, boolean x) { super(s); id = a; from = f; to = t; failed = x; }
    public AppID getAppID() { return id; }
    public int getFromState() { return from; }
    public int getToState() { return to; }
    public boolean hasFailed() { return failed; }
}
