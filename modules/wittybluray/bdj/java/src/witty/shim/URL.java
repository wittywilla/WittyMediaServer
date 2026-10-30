package witty.shim;

import java.io.ByteArrayInputStream;
import java.io.IOException;
import java.io.InputStream;
import java.net.MalformedURLException;
import witty.bdj.DiscLoader;

/** java.net.URL for disc code: the disc's own files and resources, and no network. */
public final class URL implements java.io.Serializable {
    private final String text;
    private final transient DiscLoader res;
    private final String resName;

    public URL(String s) throws MalformedURLException {
        if (s == null || s.indexOf(':') < 0) throw new MalformedURLException(s);
        text = s;
        res = null;
        resName = null;
    }
    public URL(URL ctx, String s) throws MalformedURLException { this(join(ctx, s)); }
    public URL(String proto, String host, String file) throws MalformedURLException { this(proto + "://" + host + file); }
    public URL(String proto, String host, int port, String file) throws MalformedURLException { this(proto + "://" + host + ":" + port + file); }
    URL(DiscLoader l, String name) { text = "rsrc:" + name; res = l; resName = name; }

    private static String join(URL ctx, String s) {
        if (ctx == null || s.indexOf(':') > 0) return s;
        String t = ctx.text;
        int i = t.lastIndexOf('/');
        return (i >= 0 ? t.substring(0, i + 1) : t + "/") + s;
    }
    public InputStream openStream() throws IOException {
        if (res != null) {
            byte[] b = res.read(resName);
            if (b == null) throw new java.io.FileNotFoundException(resName);
            return new ByteArrayInputStream(b);
        }
        if (text.startsWith("file:")) {
            String p = text.substring(5);
            while (p.startsWith("//")) p = p.substring(1);
            return new FileInputStream(java.net.URLDecoder.decode(p, "UTF-8"));
        }
        throw new IOException("the network is not available to disc applications: " + text);
    }
    public Object getContent() throws IOException { return openStream(); }
    public String getProtocol() { return text.substring(0, text.indexOf(':')); }
    public String getHost() { java.net.URI u = uri(); return u == null || u.getHost() == null ? "" : u.getHost(); }
    public int getPort() { java.net.URI u = uri(); return u == null ? -1 : u.getPort(); }
    public int getDefaultPort() { return -1; }
    public String getPath() { java.net.URI u = uri(); return u == null || u.getPath() == null ? text.substring(text.indexOf(':') + 1) : u.getPath(); }
    public String getFile() { java.net.URI u = uri(); if (u == null) return getPath(); return u.getRawQuery() == null ? getPath() : getPath() + "?" + u.getRawQuery(); }
    public String getQuery() { java.net.URI u = uri(); return u == null ? null : u.getRawQuery(); }
    public String getRef() { java.net.URI u = uri(); return u == null ? null : u.getRawFragment(); }
    public String getAuthority() { java.net.URI u = uri(); return u == null ? null : u.getRawAuthority(); }
    public String getUserInfo() { return null; }
    private java.net.URI uri() { try { return new java.net.URI(text); } catch (Exception e) { return null; } }
    public String toExternalForm() { return text; }
    public String toString() { return text; }
    public boolean sameFile(URL o) { return text.equals(o.text); }
    public boolean equals(Object o) { return o instanceof URL && text.equals(((URL) o).text); }
    public int hashCode() { return text.hashCode(); }
    /** Used by the platform's image loading. */
    public byte[] bytes() throws IOException {
        InputStream in = openStream();
        try {
            java.io.ByteArrayOutputStream o = new java.io.ByteArrayOutputStream();
            byte[] b = new byte[16384];
            for (int n; (n = in.read(b)) > 0; ) o.write(b, 0, n);
            return o.toByteArray();
        } finally {
            in.close();
        }
    }
}
