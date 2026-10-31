package org.bluray.media;
public class PiPStatusEvent extends java.util.EventObject {
    private final boolean available;
    public PiPStatusEvent(boolean a, Object s) { super(s); available = a; }
    public boolean getAvailable() { return available; }
}
