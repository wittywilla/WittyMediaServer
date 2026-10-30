package javax.media;
public class DurationUpdateEvent extends ControllerEvent {
    private final Time d;
    public DurationUpdateEvent(Controller c, Time v) { super(c); d = v; }
    public Time getDuration() { return d; }
}
