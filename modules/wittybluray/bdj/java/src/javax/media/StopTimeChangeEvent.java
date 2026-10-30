package javax.media;
public class StopTimeChangeEvent extends ControllerEvent {
    private final Time t;
    public StopTimeChangeEvent(Controller c, Time m) { super(c); t = m; }
    public Time getStopTime() { return t; }
}
