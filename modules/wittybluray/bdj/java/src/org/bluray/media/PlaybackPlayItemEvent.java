package org.bluray.media;
public class PlaybackPlayItemEvent extends java.util.EventObject {
    private final int item;
    public PlaybackPlayItemEvent(Object s, int i) { super(s); item = i; }
    public int getPlayItem() { return item; }
}
