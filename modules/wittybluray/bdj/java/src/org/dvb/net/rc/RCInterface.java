package org.dvb.net.rc;

/** A return channel; a disc player with the cable out has none, so none is ever handed out. */
public class RCInterface {
    public static final int TYPE_CATV = 1, TYPE_LMDS = 2, TYPE_MATV = 3, TYPE_PSTN = 4, TYPE_ISDN = 5, TYPE_DECT = 6, TYPE_SATELLITE = 7, TYPE_WLAN = 8, TYPE_OTHER = 9;
    protected RCInterface() {}
    public int getType() { return TYPE_OTHER; }
    public int getInboundMaxBitrate() { return 0; }
    public int getOutboundMaxBitrate() { return 0; }
    public boolean isReservable() { return false; }
    public boolean isConnected() { return false; }
    public String getMacAddress() { return null; }
    public int getDataRate() { return 0; }
}
