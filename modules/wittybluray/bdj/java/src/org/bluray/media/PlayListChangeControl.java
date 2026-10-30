package org.bluray.media;
public interface PlayListChangeControl extends javax.media.Control {
    void selectPlayList(org.bluray.net.BDLocator l) throws InvalidPlayListException, org.davic.net.InvalidLocatorException, org.davic.media.NotAuthorizedException;
    org.bluray.net.BDLocator getCurrentPlayList();
    void addPlayListChangeListener(PlayListChangeListener l);
    void removePlayListChangeListener(PlayListChangeListener l);
}
