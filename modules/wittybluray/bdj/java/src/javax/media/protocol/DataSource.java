package javax.media.protocol;
public abstract class DataSource implements javax.media.Controls, javax.media.Duration {
    protected javax.media.MediaLocator source;
    public DataSource() {}
    public DataSource(javax.media.MediaLocator l) { source = l; }
    public void setLocator(javax.media.MediaLocator l) { source = l; }
    public javax.media.MediaLocator getLocator() { return source; }
    public abstract String getContentType();
    public abstract void connect() throws java.io.IOException;
    public abstract void disconnect();
    public abstract void start() throws java.io.IOException;
    public abstract void stop() throws java.io.IOException;
    public Object[] getControls() { return new Object[0]; }
    public Object getControl(String type) { return null; }
    public javax.media.Time getDuration() { return DURATION_UNKNOWN; }
}
