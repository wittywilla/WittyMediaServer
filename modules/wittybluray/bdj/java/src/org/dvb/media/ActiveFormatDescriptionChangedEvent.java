package org.dvb.media;
public class ActiveFormatDescriptionChangedEvent extends VideoFormatEvent {
    public ActiveFormatDescriptionChangedEvent(Object source, int afd) { super(source, afd); }
    public int getNewValue() { return value(); }
}
