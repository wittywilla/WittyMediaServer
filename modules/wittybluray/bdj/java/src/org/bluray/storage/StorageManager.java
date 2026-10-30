package org.bluray.storage;
public class StorageManager {
    private static final StorageManager it = new StorageManager();
    private StorageManager() {}
    public static StorageManager instance() { return it; }
    public DataArea getBindingUnitDataArea() { return new DataArea(); }
    public DataArea getApplicationDataArea() { return new DataArea(); }
}
