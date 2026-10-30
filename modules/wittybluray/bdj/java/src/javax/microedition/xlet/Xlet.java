package javax.microedition.xlet;
public interface Xlet {
    void initXlet(XletContext ctx) throws XletStateChangeException;
    void startXlet() throws XletStateChangeException;
    void pauseXlet();
    void destroyXlet(boolean unconditional) throws XletStateChangeException;
}
