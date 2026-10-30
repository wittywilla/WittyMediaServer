package org.bluray.storage;
public class DataArea {
    public long getTotalSpace() { return 64L << 20; }
    public long getFreeSpace() { return 32L << 20; }
    public void addListener(Object l) {}
    public void removeListener(Object l) {}
}
