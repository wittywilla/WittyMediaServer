package javax.media;
public class MediaTimeSetEvent extends ControllerEvent {
    private final Time t;
    public MediaTimeSetEvent(Controller c, Time m) { super(c); t = m; }
    public Time getMediaTime() { return t; }
}
