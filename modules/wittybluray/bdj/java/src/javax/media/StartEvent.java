package javax.media;
public class StartEvent extends TransitionEvent {
    private final Time media, tb;
    public StartEvent(Controller c, int p, int s, int t, Time m, Time b) { super(c, p, s, t); media = m; tb = b; }
    public Time getMediaTime() { return media; }
    public Time getTimeBaseTime() { return tb; }
}
