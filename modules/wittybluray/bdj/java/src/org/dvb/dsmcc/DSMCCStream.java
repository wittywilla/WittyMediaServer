package org.dvb.dsmcc;

/** A stream object; the disc has none, so it never carries events. */
public class DSMCCStream extends DSMCCObject {
    public DSMCCStream(String path) { super(path); }
    public DSMCCStream(String path, String name) { super(path, name); }
    public DSMCCStream(DSMCCObject dir, String name) { super(dir, name); }
    public boolean isStream() { return true; }
    public javax.media.Time getDuration() { return javax.media.Duration.DURATION_UNKNOWN; }
}
