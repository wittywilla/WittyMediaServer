package org.dvb.media;
public interface DVBMediaSelectControl extends javax.tv.media.MediaSelectControl {
    void selectServiceMediaHandler(javax.tv.locator.Locator l) throws org.davic.net.InvalidLocatorException, javax.tv.service.selection.InvalidServiceComponentException, org.davic.media.NotAuthorizedException;
}
