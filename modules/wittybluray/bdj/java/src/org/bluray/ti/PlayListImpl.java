package org.bluray.ti;
final class PlayListImpl implements PlayList {
    private final int id;
    PlayListImpl(int i) { id = i; }
    public int getId() { return id; }
    public org.bluray.net.BDLocator getLocator() {
        try {
            return new org.bluray.net.BDLocator(null, -1, id, -1, -1, null);
        } catch (org.davic.net.InvalidLocatorException e) {
            return null;
        }
    }
    public PlayItem[] getPlayItems() {
        String name = Integer.toString(id);
        while (name.length() < 5) name = "0" + name;
        // The playlist file: its table starts where byte 8 says, and gives the count of items six bytes in.
        byte[] b = DiscFiles.head("BDMV/PLAYLIST/" + name + ".mpls", 4096);
        int n = 0;
        if (b != null && b.length >= 12) {
            int at = DiscFiles.u32(b, 8) + 6;
            if (at >= 0 && at + 2 <= b.length) n = DiscFiles.u16(b, at);
        }
        PlayItem[] o = new PlayItem[n];
        for (int i = 0; i < n; i++) o[i] = new Item(i);
        return o;
    }
    private final class Item implements PlayItem {
        private final int item;
        Item(int i) { item = i; }
        public int getId() { return item; }
        public PlayList getPlayList() { return PlayListImpl.this; }
    }
}
