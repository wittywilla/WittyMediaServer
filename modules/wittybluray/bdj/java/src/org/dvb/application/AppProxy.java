package org.dvb.application;
public interface AppProxy {
    int NOT_LOADED = 1, PAUSED = 2, STARTED = 3, DESTROYED = 4;
    int getState();
    void start();
    void start(String[] args);
    void stop(boolean forced);
    void pause();
    void resume();
    void addAppStateChangeEventListener(AppStateChangeEventListener l);
    void removeAppStateChangeEventListener(AppStateChangeEventListener l);
}
