package org.dvb.application;
public abstract class AppsDatabaseFilter {
    public AppsDatabaseFilter() {}
    public abstract boolean accept(AppID id);
}
