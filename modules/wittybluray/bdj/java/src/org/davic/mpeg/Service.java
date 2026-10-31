package org.davic.mpeg;
public class Service {
    private final int id;
    protected Service(int id) { this.id = id; }
    public int getServiceId() { return id; }
    public ElementaryStream[] retrieveElementaryStreams() { return new ElementaryStream[0]; }
}
