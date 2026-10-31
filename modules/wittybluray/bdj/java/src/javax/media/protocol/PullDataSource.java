package javax.media.protocol;
public abstract class PullDataSource extends DataSource {
    public PullDataSource() {}
    public PullDataSource(javax.media.MediaLocator l) { super(l); }
    public abstract PullSourceStream[] getStreams();
}
