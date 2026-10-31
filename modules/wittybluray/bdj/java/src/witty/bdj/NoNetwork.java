package witty.bdj;

import java.io.FileDescriptor;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.InetAddress;
import java.net.ServerSocket;
import java.net.Socket;
import java.net.SocketAddress;
import java.net.SocketException;
import java.net.SocketImpl;
import java.net.SocketImplFactory;
import java.net.URL;
import java.net.URLConnection;
import java.net.URLStreamHandler;
import java.net.URLStreamHandlerFactory;

/**
 * A second wall behind the allow list: should a class ever slip through it, no
 * socket of this process connects and no URL handler reaches a network, whoever
 * asks. The runtime needs neither; it talks to the server over a pipe.
 */
final class NoNetwork {
    private NoNetwork() {}

    private static final class Refused extends SocketImpl {
        private static SocketException no() { return new SocketException("the network is not available to disc applications"); }
        protected void create(boolean stream) throws IOException { throw no(); }
        protected void connect(String host, int port) throws IOException { throw no(); }
        protected void connect(InetAddress a, int port) throws IOException { throw no(); }
        protected void connect(SocketAddress a, int timeout) throws IOException { throw no(); }
        protected void bind(InetAddress a, int port) throws IOException { throw no(); }
        protected void listen(int backlog) throws IOException { throw no(); }
        protected void accept(SocketImpl s) throws IOException { throw no(); }
        protected InputStream getInputStream() throws IOException { throw no(); }
        protected OutputStream getOutputStream() throws IOException { throw no(); }
        protected int available() throws IOException { throw no(); }
        protected void close() {}
        protected void sendUrgentData(int d) throws IOException { throw no(); }
        public void setOption(int id, Object v) throws SocketException { throw no(); }
        public Object getOption(int id) throws SocketException { throw no(); }
    }

    static void install() throws IOException {
        SocketImplFactory f = new SocketImplFactory() {
            public SocketImpl createSocketImpl() { return new Refused(); }
        };
        Socket.setSocketImplFactory(f);
        ServerSocket.setSocketFactory(f);
        URL.setURLStreamHandlerFactory(new URLStreamHandlerFactory() {
            public URLStreamHandler createURLStreamHandler(String protocol) {
                if (protocol.equals("file") || protocol.equals("jar") || protocol.equals("jrt")) return null; // how the JDK loads its own classes
                return new URLStreamHandler() {
                    protected URLConnection openConnection(URL u) throws IOException { throw new IOException("the network is not available to disc applications"); }
                };
            }
        });
    }
}
