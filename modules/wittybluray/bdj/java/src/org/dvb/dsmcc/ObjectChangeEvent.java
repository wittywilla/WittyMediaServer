package org.dvb.dsmcc;
public class ObjectChangeEvent extends java.util.EventObject {
    private final int version;
    public ObjectChangeEvent(Object s) { this(s, 0); }
    public ObjectChangeEvent(Object s, int v) { super(s); version = v; }
    public int getNewVersion() { return version; }
}
