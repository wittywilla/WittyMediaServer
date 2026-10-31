package org.dvb.dsmcc;
public class StreamEvent extends java.util.EventObject {
    private final int id;
    private final String name;
    private final byte[] data;
    private final long npt;
    public StreamEvent(Object s, int id, String name, byte[] data, long npt) { super(s); this.id = id; this.name = name; this.data = data == null ? new byte[0] : data.clone(); this.npt = npt; }
    public StreamEvent(Object s, int id, byte[] data, long npt) { this(s, id, "", data, npt); }
    public int getEventId() { return id; }
    public String getEventName() { return name; }
    public byte[] getEventData() { return data.clone(); }
    public long getEventNPT() { return npt; }
}
