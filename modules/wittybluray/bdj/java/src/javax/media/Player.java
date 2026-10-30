package javax.media;
public interface Player extends MediaHandler, Controller {
    java.awt.Component getVisualComponent();
    GainControl getGainControl();
    java.awt.Component getControlPanelComponent();
    void start();
    void addController(Controller c) throws IncompatibleTimeBaseException;
    void removeController(Controller c);
}
