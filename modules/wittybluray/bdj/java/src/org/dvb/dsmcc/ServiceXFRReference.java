package org.dvb.dsmcc;
public class ServiceXFRReference {
    private final byte[] nsap;
    private final String path;
    public ServiceXFRReference(byte[] nsap, String path) { this.nsap = nsap == null ? new byte[0] : nsap.clone(); this.path = path; }
    public byte[] getNSAPAddress() { return nsap.clone(); }
    public String getPathName() { return path; }
}
