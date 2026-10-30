package witty.shim;

import java.net.UnknownHostException;

public class InetAddress {
    private InetAddress() {}
    public static InetAddress getByName(String h) throws UnknownHostException { throw new UnknownHostException(h); }
    public static InetAddress[] getAllByName(String h) throws UnknownHostException { throw new UnknownHostException(h); }
    public static InetAddress getLocalHost() throws UnknownHostException { throw new UnknownHostException("localhost"); }
    public String getHostAddress() { return ""; }
    public String getHostName() { return ""; }
}
