package org.bluray.vfs;
/** Combining downloaded content with the disc needs BD-Live, which has no network here. */
public class VFSManager {
    private static final VFSManager it = new VFSManager();
    public static final int STABLE = 1, PREPARING = 2, PREPARED = 3, UPDATING = 4;
    private VFSManager() {}
    public static VFSManager getInstance() { return it; }
    public int getState() { return STABLE; }
    public void requestUpdating(String xml, String sig, boolean x) throws Exception { throw new SecurityException("downloaded content is not supported"); }
    public void addListener(Object l) {}
    public void removeListener(Object l) {}
}
