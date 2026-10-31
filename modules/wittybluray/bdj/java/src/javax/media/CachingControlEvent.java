package javax.media;
public class CachingControlEvent extends ControllerEvent {
    private final CachingControl control;
    private final long progress;
    public CachingControlEvent(Controller from, CachingControl cc, long progress) { super(from); control = cc; this.progress = progress; }
    public CachingControl getCachingControl() { return control; }
    public long getContentProgress() { return progress; }
}
