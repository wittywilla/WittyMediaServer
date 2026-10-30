package javax.media.protocol;
public abstract class DataSource {
    private javax.media.MediaLocator loc;
    public DataSource() {}
    public DataSource(javax.media.MediaLocator l) { loc = l; }
    public void setLocator(javax.media.MediaLocator l) { loc = l; }
    public javax.media.MediaLocator getLocator() { return loc; }
    public abstract String getContentType();
    public abstract void connect() throws java.io.IOException;
    public abstract void disconnect();
    public abstract void start() throws java.io.IOException;
    public abstract void stop() throws java.io.IOException;
}
