package org.dvb.media;

/** Media pushed in by the application; with no broadcast decoder behind it the data is taken and dropped. */
public class DripFeedDataSource extends javax.media.protocol.DataSource {
    private volatile boolean connected, started;
    public DripFeedDataSource() {}
    public void feed(byte[] data) { if (data == null) throw new NullPointerException(); }
    public String getContentType() { return "video.mpeg"; }
    public void connect() throws java.io.IOException { connected = true; }
    public void disconnect() { connected = false; started = false; }
    public void start() throws java.io.IOException { if (!connected) throw new java.io.IOException("not connected"); started = true; }
    public void stop() throws java.io.IOException { started = false; }
    public Object[] getControls() { return new Object[0]; }
    public Object getControl(String type) { return null; }
    public javax.media.Time getDuration() { return javax.media.Duration.DURATION_UNKNOWN; }
}
