package org.dvb.application;
public class AppsDatabaseEvent extends java.util.EventObject {
    public static final int NEW_DATABASE = 0, APP_ADDED = 1, APP_CHANGED = 2, APP_DELETED = 3;
    private final int id;
    private final AppID app;
    public AppsDatabaseEvent(int i, AppID a, Object s) { super(s); id = i; app = a; }
    public AppID getAppID() { return app; }
    public int getEventId() { return id; }
}
