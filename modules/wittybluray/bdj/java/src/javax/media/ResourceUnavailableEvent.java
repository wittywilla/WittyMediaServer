package javax.media;
public class ResourceUnavailableEvent extends ControllerErrorEvent {
    public ResourceUnavailableEvent(Controller c) { super(c); }
    public ResourceUnavailableEvent(Controller c, String m) { super(c, m); }
}
