package org.dvb.dsmcc;

public class DSMCCStreamEvent extends DSMCCStream {
    public DSMCCStreamEvent(String path) { super(path); }
    public DSMCCStreamEvent(String path, String name) { super(path, name); }
    public DSMCCStreamEvent(DSMCCObject dir, String name) { super(dir, name); }
    public boolean isStreamEvent() { return true; }
    public String[] getEventList() { return new String[0]; }
    public int subscribe(String name, StreamEventListener l) throws UnknownEventException, InsufficientResourcesException, MPEGDeliveryException { throw new UnknownEventException(name); }
    public int subscribe(int id, StreamEventListener l) throws UnknownEventException, InsufficientResourcesException, MPEGDeliveryException { throw new UnknownEventException(String.valueOf(id)); }
    public void unsubscribe(int id, StreamEventListener l) throws UnknownEventException { throw new UnknownEventException(String.valueOf(id)); }
    public void unsubscribe(String name, StreamEventListener l) throws UnknownEventException { throw new UnknownEventException(name); }
}
