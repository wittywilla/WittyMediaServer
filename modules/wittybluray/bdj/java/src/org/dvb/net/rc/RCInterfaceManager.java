package org.dvb.net.rc;

public class RCInterfaceManager implements org.davic.resources.ResourceServer {
    private static final RCInterfaceManager it = new RCInterfaceManager();
    protected RCInterfaceManager() {}
    public static RCInterfaceManager getInstance() { return it; }
    public RCInterface[] getInterfaces() { return new RCInterface[0]; }
    public RCInterface getInterface(String macAddress) { return null; }
    public void addResourceStatusEventListener(org.davic.resources.ResourceStatusListener l) {}
    public void removeResourceStatusEventListener(org.davic.resources.ResourceStatusListener l) {}
}
