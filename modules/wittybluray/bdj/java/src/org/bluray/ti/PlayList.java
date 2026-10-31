package org.bluray.ti;
public interface PlayList {
    int getId();
    org.bluray.net.BDLocator getLocator();
    PlayItem[] getPlayItems();
}
