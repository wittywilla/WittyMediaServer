package a;

/** Probes that name a class disc code must not be able to link: kept apart, so that only calling one fails. */
final class Blocked {
    private Blocked() {}
    static void serverSocket(String port) throws Exception { new java.net.ServerSocket(Integer.parseInt(port)); }
    static void datagram(String port) throws Exception {
        java.net.DatagramSocket d = new java.net.DatagramSocket();
        d.connect(java.net.InetSocketAddress.createUnresolved("127.0.0.1", Integer.parseInt(port)));
    }
    static void channel(String port) throws Exception { java.nio.channels.SocketChannel.open(new java.net.InetSocketAddress("127.0.0.1", Integer.parseInt(port))); }
    static void urlConnection(String u) throws Exception { new java.net.URL(u).openConnection().connect(); }
    static boolean nio(String p) throws Exception { return java.nio.file.Files.readAllBytes(java.nio.file.Paths.get(p)).length > 0; }
    static boolean descriptor() throws Exception { return java.io.FileDescriptor.out != null; }
    static boolean zipFile(String p) throws Exception { new java.util.zip.ZipFile(p); return true; }
    static boolean printStream(String p) throws Exception { new java.io.PrintStream(p).close(); return true; }
    static boolean host() { witty.bdj.Host.log("a disc reached the link"); return true; }
    static boolean ownLoader() throws Exception { return new Own().make() != null; }
    private static final class Own extends ClassLoader {
        Class<?> make() { byte[] b = new byte[8]; return defineClass("x.Y", b, 0, b.length); }
    }
    static boolean reflect() throws Exception { return String.class.getMethod("length").invoke("x") != null; }
    static boolean handles() throws Exception { return java.lang.invoke.MethodHandles.lookup() != null; }
    static boolean proxy() throws Exception {
        return java.lang.reflect.Proxy.newProxyInstance(null, new Class[] {Runnable.class}, new java.lang.reflect.InvocationHandler() {
            public Object invoke(Object o, java.lang.reflect.Method m, Object[] a) { return null; }
        }) != null;
    }
    static boolean serial() throws Exception { return new java.io.ObjectInputStream(new java.io.ByteArrayInputStream(new byte[4])) != null; }
    static boolean process() throws Exception { new ProcessBuilder("cmd").start(); return true; }
    static boolean exec() throws Exception { Runtime.getRuntime().exec("cmd"); return true; }
    static boolean desktop() throws Exception { return java.awt.Desktop.isDesktopSupported(); }
}
