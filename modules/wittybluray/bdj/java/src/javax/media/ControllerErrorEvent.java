package javax.media;
public class ControllerErrorEvent extends ControllerClosedEvent {
    public ControllerErrorEvent(Controller c) { super(c); }
    public ControllerErrorEvent(Controller c, String m) { super(c, m); }
}
