package javax.media.protocol;
public abstract class PushDataSource extends DataSource {
    public PushDataSource() {}
    public PushDataSource(javax.media.MediaLocator l) { super(l); }
    public abstract PushSourceStream[] getStreams();
}
