package org.havi.ui;
public class HScreenDevice implements org.davic.resources.ResourceProxy, org.davic.resources.ResourceServer {
    protected HScreenDevice() {}
    public String getIDstring() { return "WittyBluray"; }
    public java.awt.Dimension getScreenAspectRatio() { return new java.awt.Dimension(16, 9); }
    public boolean reserveDevice(org.davic.resources.ResourceClient c) { return true; }
    public void releaseDevice() {}
    public org.davic.resources.ResourceClient getClient() { return null; }
    public void addResourceStatusEventListener(org.davic.resources.ResourceStatusListener l) {}
    public void removeResourceStatusEventListener(org.davic.resources.ResourceStatusListener l) {}
    public void addScreenConfigurationListener(org.havi.ui.event.HScreenConfigurationListener l) {}
    public void addScreenConfigurationListener(org.havi.ui.event.HScreenConfigurationListener l, HScreenConfigTemplate t) {}
    public void removeScreenConfigurationListener(org.havi.ui.event.HScreenConfigurationListener l) {}
}
