package javax.tv.xlet;
public interface XletContext {
    String ARGS = "javax.tv.xlet.args";
    void notifyDestroyed();
    void notifyPaused();
    Object getXletProperty(String key);
    void resumeRequest();
}
