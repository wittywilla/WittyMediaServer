package org.dvb.media;
public class DFCChangedEvent extends VideoFormatEvent {
    public DFCChangedEvent(Object source, int dfc) { super(source, dfc); }
    public int getNewValue() { return value(); }
}
