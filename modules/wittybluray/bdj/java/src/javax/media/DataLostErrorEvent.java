package javax.media;
public class DataLostErrorEvent extends ControllerClosedEvent {
    public DataLostErrorEvent(Controller from) { super(from); }
    public DataLostErrorEvent(Controller from, String why) { super(from, why); }
}
