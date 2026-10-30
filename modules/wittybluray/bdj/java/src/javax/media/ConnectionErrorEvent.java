package javax.media;
public class ConnectionErrorEvent extends ControllerErrorEvent {
    public ConnectionErrorEvent(Controller c) { super(c); }
    public ConnectionErrorEvent(Controller c, String m) { super(c, m); }
}
