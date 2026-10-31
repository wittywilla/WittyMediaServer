package org.bluray.vfs;

import java.util.HashSet;
import java.util.Set;

/** Combining downloaded content with the disc needs BD-Live, which has no network here: the disc's own clips are all there is. */
public class VFSManager {
    private static final VFSManager it = new VFSManager();
    public static final int STABLE = 1, PREPARING = 2, PREPARED = 3, UPDATING = 4;
    private final Set<String> disabled = new HashSet<String>();
    private VFSManager() {}
    public static VFSManager getInstance() { return it; }
    public int getState() { return STABLE; }
    public void requestUpdating(String xml, String sig, boolean x) throws PreparingFailedException { throw new PreparingFailedException("downloaded content is not supported"); }
    private static boolean clip(String id) { return id != null && id.matches("[0-9]{5}"); }
    public synchronized boolean isEnabledClip(String id) { return clip(id) && !disabled.contains(id); }
    public synchronized void enableClip(String id) { disabled.remove(id); }
    public synchronized void disableClip(String id) { if (clip(id)) disabled.add(id); }
    public synchronized String[] getDisabledClipIDs() { return disabled.toArray(new String[disabled.size()]); }
    public void addListener(Object l) {}
    public void removeListener(Object l) {}
}
