package javax.tv.net;
/** Maps a locator to the local network interface that carries it: a disc player has no broadcast interfaces, so nothing maps. */
public class InterfaceMap {
    private InterfaceMap() {}
    public static witty.shim.InetAddress getInterface(javax.tv.locator.Locator l) { return null; }
}
