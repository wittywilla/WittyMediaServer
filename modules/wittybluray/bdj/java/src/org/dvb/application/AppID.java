package org.dvb.application;
public class AppID {
    private final int oid, aid;
    public AppID(int o, int a) { oid = o; aid = a; }
    public int getOID() { return oid; }
    public int getAID() { return aid; }
    public boolean equals(Object o) { return o instanceof AppID && ((AppID) o).oid == oid && ((AppID) o).aid == aid; }
    public int hashCode() { return oid * 31 + aid; }
    public String toString() { return Integer.toHexString(oid) + "-" + Integer.toHexString(aid); }
}
