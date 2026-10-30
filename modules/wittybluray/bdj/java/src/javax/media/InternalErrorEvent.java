package javax.media;
public class InternalErrorEvent extends ControllerErrorEvent {
    public InternalErrorEvent(Controller c) { super(c); }
    public InternalErrorEvent(Controller c, String m) { super(c, m); }
}
