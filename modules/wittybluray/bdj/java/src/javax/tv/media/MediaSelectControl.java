package javax.tv.media;
public interface MediaSelectControl extends javax.media.Control {
    void select(javax.tv.locator.Locator component) throws javax.tv.service.selection.InvalidServiceComponentException, javax.tv.service.selection.InsufficientResourcesException, SecurityException;
    void select(javax.tv.locator.Locator[] components) throws javax.tv.service.selection.InvalidServiceComponentException, javax.tv.service.selection.InsufficientResourcesException, SecurityException;
    void add(javax.tv.locator.Locator component) throws javax.tv.service.selection.InvalidServiceComponentException, javax.tv.service.selection.InsufficientResourcesException, SecurityException;
    void remove(javax.tv.locator.Locator component) throws javax.tv.service.selection.InvalidServiceComponentException, SecurityException;
    void replace(javax.tv.locator.Locator from, javax.tv.locator.Locator to) throws javax.tv.service.selection.InvalidServiceComponentException, javax.tv.service.selection.InsufficientResourcesException, SecurityException;
    javax.tv.locator.Locator[] getCurrentSelection();
    void addMediaSelectListener(MediaSelectListener l);
    void removeMediaSelectListener(MediaSelectListener l);
}
