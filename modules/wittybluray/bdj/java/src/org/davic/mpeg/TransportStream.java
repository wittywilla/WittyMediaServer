package org.davic.mpeg;
public class TransportStream {
    private final int id;
    protected TransportStream(int id) { this.id = id; }
    public int getTransportStreamId() { return id; }
    public Service[] retrieveServices() { return new Service[0]; }
}
