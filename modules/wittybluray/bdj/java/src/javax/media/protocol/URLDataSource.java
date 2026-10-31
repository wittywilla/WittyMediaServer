package javax.media.protocol;
/** Reads a file or resource of the disc through the sandboxed URL; there is no network. */
public class URLDataSource extends PullDataSource {
    protected witty.shim.URL url;
    protected ContentDescriptor contentType = new ContentDescriptor(ContentDescriptor.CONTENT_UNKNOWN);
    private java.io.InputStream in;
    private PullSourceStream[] streams = new PullSourceStream[0];

    public URLDataSource() {}
    public URLDataSource(witty.shim.URL u) throws java.io.IOException {
        url = u;
        if (u != null) super.setLocator(new javax.media.MediaLocator(u));
        contentType = new ContentDescriptor(ContentDescriptor.mimeTypeToPackageName(guess(u == null ? "" : u.toExternalForm())));
    }
    public void setLocator(javax.media.MediaLocator l) {
        super.setLocator(l);
        try {
            url = l == null ? null : l.getURL();
        } catch (java.net.MalformedURLException e) {
            url = null;
        }
        contentType = new ContentDescriptor(ContentDescriptor.mimeTypeToPackageName(guess(l == null ? "" : l.toExternalForm())));
    }
    private static String guess(String n) {
        n = n.toLowerCase();
        if (n.endsWith(".mpg") || n.endsWith(".mpeg")) return "video/mpeg";
        if (n.endsWith(".mp3")) return "audio/mpeg";
        if (n.endsWith(".wav")) return "audio/x-wav";
        if (n.endsWith(".au")) return "audio/basic";
        if (n.endsWith(".avi")) return "video/x_msvideo";
        if (n.endsWith(".gif")) return "image/gif";
        if (n.endsWith(".jpg") || n.endsWith(".jpeg")) return "image/jpeg";
        if (n.endsWith(".png")) return "image/png";
        return ContentDescriptor.CONTENT_UNKNOWN;
    }
    public String getContentType() { return contentType.getContentType(); }
    public synchronized void connect() throws java.io.IOException {
        if (in != null) return;
        if (url == null) throw new java.io.IOException("no URL");
        in = url.openStream();
        streams = new PullSourceStream[] {new Stream(in, contentType)};
    }
    public synchronized void disconnect() {
        java.io.InputStream i = in;
        in = null;
        streams = new PullSourceStream[0];
        if (i != null) {
            try {
                i.close();
            } catch (java.io.IOException e) {
            }
        }
    }
    public void start() {}
    public void stop() {}
    public synchronized PullSourceStream[] getStreams() { return streams.clone(); }

    private static final class Stream implements PullSourceStream {
        private final java.io.InputStream in;
        private final ContentDescriptor type;
        private volatile boolean eos;
        Stream(java.io.InputStream i, ContentDescriptor t) { in = i; type = t; }
        public boolean willReadBlock() {
            try {
                return !eos && in.available() <= 0;
            } catch (java.io.IOException e) {
                return false;
            }
        }
        public int read(byte[] b, int off, int len) throws java.io.IOException {
            int n = in.read(b, off, len);
            if (n < 0) eos = true;
            return n;
        }
        public ContentDescriptor getContentDescriptor() { return type; }
        public long getContentLength() { return LENGTH_UNKNOWN; }
        public boolean endOfStream() { return eos; }
        public Object[] getControls() { return new Object[0]; }
        public Object getControl(String t) { return null; }
    }
}
