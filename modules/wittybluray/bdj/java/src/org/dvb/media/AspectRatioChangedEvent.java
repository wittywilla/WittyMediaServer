package org.dvb.media;
public class AspectRatioChangedEvent extends VideoFormatEvent {
    public AspectRatioChangedEvent(Object source, int ratio) { super(source, ratio); }
    public int getNewValue() { return value(); }
}
