package org.dvb.event;

import java.util.ArrayList;
import java.util.List;
import witty.bdj.Keys;

/** Applications ask here for the keys they want; they get them before any scene does. */
public class EventManager implements org.davic.resources.ResourceServer {
    private static final EventManager it = new EventManager();
    private static final class Entry {
        UserEventListener l;
        UserEventRepository r;
        boolean exclusive;
        org.davic.resources.ResourceClient client;
    }
    private final List<Entry> entries = new java.util.concurrent.CopyOnWriteArrayList<Entry>();
    protected EventManager() {
        Keys.init();
        Keys.addFirstSink(new Keys.Sink() {
            public boolean key(int code, int id, long when) {
                boolean taken = false;
                for (Entry e : entries) {
                    if (!e.r.has(code)) continue;
                    if (e.l != null) e.l.userEventReceived(new UserEvent(e.r, UserEvent.UEF_KEY_EVENT, id, code, 0, when));
                    taken = taken || e.exclusive;
                }
                return taken;
            }
        });
    }
    public static EventManager getInstance() { return it; }
    public boolean addUserEventListener(UserEventListener l, org.davic.resources.ResourceClient c, UserEventRepository r) { return add(l, c, r, true); }
    public void addUserEventListener(UserEventListener l, UserEventRepository r) { add(l, null, r, false); }
    private boolean add(UserEventListener l, org.davic.resources.ResourceClient c, UserEventRepository r, boolean ex) {
        Entry e = new Entry();
        e.l = l;
        e.r = r;
        e.exclusive = ex;
        e.client = c;
        entries.add(e);
        return true;
    }
    public void removeUserEventListener(UserEventListener l) { for (Entry e : entries) if (e.l == l) entries.remove(e); }
    public boolean addExclusiveAccessToAWTEvent(org.davic.resources.ResourceClient c, UserEventRepository r) { return add(null, c, r, true); }
    public void removeExclusiveAccessToAWTEvent(org.davic.resources.ResourceClient c) { for (Entry e : entries) if (e.client == c) entries.remove(e); }
    public void addResourceStatusEventListener(org.davic.resources.ResourceStatusListener l) {}
    public void removeResourceStatusEventListener(org.davic.resources.ResourceStatusListener l) {}
}
