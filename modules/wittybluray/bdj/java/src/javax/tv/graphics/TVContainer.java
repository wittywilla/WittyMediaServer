package javax.tv.graphics;
public class TVContainer {
    private TVContainer() {}
    public static java.awt.Container getRootContainer(javax.tv.xlet.XletContext ctx) { return org.havi.ui.HSceneFactory.getInstance().getDefaultHScene(); }
}
