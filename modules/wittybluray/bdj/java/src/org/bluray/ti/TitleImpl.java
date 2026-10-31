package org.bluray.ti;
public class TitleImpl implements Title {
    private final int n;
    public TitleImpl(int number) { n = number; }
    public int getTitleNumber() { return n; }
    public String getName() { return "Title " + n; }
    public javax.tv.locator.Locator getLocator() {
        try {
            return new org.bluray.net.BDLocator("bd://" + Integer.toHexString(n));
        } catch (org.davic.net.InvalidLocatorException e) {
            return null;
        }
    }
    /** Every playlist of the disc: which of them a title plays is decided by its program, not written down. */
    public PlayList[] getPlayLists() {
        String[] names = new witty.shim.File(witty.bdj.Host.vfsRoot + java.io.File.separator + "BDMV" + java.io.File.separator + "PLAYLIST").list();
        if (names == null) return new PlayList[0];
        java.util.List<PlayList> o = new java.util.ArrayList<PlayList>();
        for (int i = 0; i < names.length; i++) {
            if (!names[i].matches("[0-9]{5}\\.(?i:mpls)")) continue;
            o.add(new PlayListImpl(Integer.parseInt(names[i].substring(0, 5))));
        }
        return o.toArray(new PlayList[o.size()]);
    }
    public boolean hasMultipleInstances() { return false; }
    public javax.tv.service.ServiceType getServiceType() { return javax.tv.service.ServiceType.UNKNOWN; }
    public boolean equals(Object o) { return o instanceof TitleImpl && ((TitleImpl) o).n == n; }
    public int hashCode() { return n; }
}
