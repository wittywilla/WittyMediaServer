package javax.media;
public class ControllerEvent extends java.util.EventObject {
    public ControllerEvent(Controller c) { super(c); }
    public Controller getSourceController() { return (Controller) getSource(); }
}
