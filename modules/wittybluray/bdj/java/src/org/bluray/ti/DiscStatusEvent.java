package org.bluray.ti;
public class DiscStatusEvent extends java.util.EventObject {
    public static final int DISC_INSERTED = 1, DISC_EJECTED = 2;
    private final Disc disc;
    private final int status;
    public DiscStatusEvent(Object s, Disc d, int st) { super(s); disc = d; status = st; }
    public Disc getDisc() { return disc; }
    public int getStatus() { return status; }
}
