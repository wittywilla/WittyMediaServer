package witty.shim;

import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;

/** There is no network: BD-Live discs see what they would see with the cable out. */
public class Socket {
    private static IOException no() { return new java.net.ConnectException("the network is not available to disc applications"); }
    public Socket() {}
    public Socket(String host, int port) throws IOException { throw no(); }
    public Socket(InetAddress a, int port) throws IOException { throw no(); }
    public InputStream getInputStream() throws IOException { throw no(); }
    public OutputStream getOutputStream() throws IOException { throw no(); }
    public void close() {}
    public boolean isConnected() { return false; }
    public void setSoTimeout(int t) {}
    public void setTcpNoDelay(boolean b) {}
}
