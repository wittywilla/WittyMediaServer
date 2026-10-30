package javax.media;
public class TransitionEvent extends ControllerEvent {
    private final int prev, cur, tgt;
    public TransitionEvent(Controller c, int p, int s, int t) { super(c); prev = p; cur = s; tgt = t; }
    public int getPreviousState() { return prev; }
    public int getCurrentState() { return cur; }
    public int getTargetState() { return tgt; }
}
