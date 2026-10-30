package javax.microedition.xlet;
public interface XletContext {
    String ARGS = "javax.microedition.xlet.args";
    void notifyDestroyed();
    void notifyPaused();
    Object getXletProperty(String key);
    void resumeRequest();
    java.awt.Container getContainer() throws UnavailableContainerException;
}
