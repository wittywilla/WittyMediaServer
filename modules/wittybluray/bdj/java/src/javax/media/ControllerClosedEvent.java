package javax.media;
public class ControllerClosedEvent extends ControllerEvent {
    protected String message = "";
    public ControllerClosedEvent(Controller c) { super(c); }
    public ControllerClosedEvent(Controller c, String m) { super(c); message = m; }
    public String getMessage() { return message; }
}
