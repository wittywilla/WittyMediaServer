package org.havi.ui;

import java.util.List;
import org.davic.resources.ResourceClient;
import org.davic.resources.ResourceStatusEvent;
import org.davic.resources.ResourceStatusListener;

/** There is one of each device and one configuration of each, so a reservation is always granted and a configuration never changes. */
public class HScreenDevice implements org.davic.resources.ResourceProxy, org.davic.resources.ResourceServer {
    private final List<ResourceStatusListener> ls = new java.util.concurrent.CopyOnWriteArrayList<ResourceStatusListener>();
    private volatile ResourceClient client;
    protected HScreenDevice() {}
    public String getIDstring() { return "WittyBluray"; }
    public java.awt.Dimension getScreenAspectRatio() { return new java.awt.Dimension(16, 9); }
    public boolean reserveDevice(ResourceClient c) {
        if (c == null) throw new NullPointerException();
        final ResourceClient old = client;
        client = c;
        if (old != null && old != c) {
            HComponent.later(new Runnable() { public void run() { old.notifyRelease(HScreenDevice.this); } });
        }
        if (old != c) status(new HScreenDeviceReservedEvent(this));
        return true;
    }
    public void releaseDevice() {
        if (client == null) return;
        client = null;
        status(new HScreenDeviceReleasedEvent(this));
    }
    public ResourceClient getClient() { return client; }
    private void status(final ResourceStatusEvent e) {
        if (ls.isEmpty()) return;
        HComponent.later(new Runnable() { public void run() { for (ResourceStatusListener l : ls) l.statusChanged(e); } });
    }
    public void addResourceStatusEventListener(ResourceStatusListener l) { if (l != null) ls.add(l); }
    public void removeResourceStatusEventListener(ResourceStatusListener l) { ls.remove(l); }
    public void addScreenConfigurationListener(org.havi.ui.event.HScreenConfigurationListener l) {}
    public void addScreenConfigurationListener(org.havi.ui.event.HScreenConfigurationListener l, HScreenConfigTemplate t) {}
    public void removeScreenConfigurationListener(org.havi.ui.event.HScreenConfigurationListener l) {}
}
