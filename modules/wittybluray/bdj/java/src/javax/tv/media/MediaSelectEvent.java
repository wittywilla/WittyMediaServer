package javax.tv.media;
public abstract class MediaSelectEvent extends javax.media.ControllerEvent {
    private final javax.tv.locator.Locator[] sel;
    public MediaSelectEvent(javax.media.Controller source, javax.tv.locator.Locator[] selection) { super(source); sel = selection == null ? new javax.tv.locator.Locator[0] : selection.clone(); }
    public javax.tv.locator.Locator[] getSelection() { return sel.clone(); }
}
