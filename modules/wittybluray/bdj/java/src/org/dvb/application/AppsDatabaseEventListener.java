package org.dvb.application;
public interface AppsDatabaseEventListener extends java.util.EventListener {
    void newDatabase(AppsDatabaseEvent e);
    void entryAdded(AppsDatabaseEvent e);
    void entryRemoved(AppsDatabaseEvent e);
    void entryChanged(AppsDatabaseEvent e);
}
