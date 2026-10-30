package javax.media;
public class StopEvent extends TransitionEvent {
    private final Time media;
    public StopEvent(Controller c, int p, int s, int t, Time m) { super(c, p, s, t); media = m; }
    public Time getMediaTime() { return media; }
}
