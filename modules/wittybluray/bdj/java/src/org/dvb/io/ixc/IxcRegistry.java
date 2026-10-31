package org.dvb.io.ixc;

import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

/** Applications on one disc sharing objects: each binds under its own organisation and application ids, and finds others by /org/app/name. */
public class IxcRegistry {
    private static final Map<String, java.rmi.Remote> reg = new HashMap<String, java.rmi.Remote>();

    private IxcRegistry() {}
    private static String hex(Object o) {
        String s = String.valueOf(o).toLowerCase();
        try { return Long.toHexString(Long.parseLong(s, 16)); } catch (NumberFormatException e) { return s; }
    }
    private static String owner(javax.tv.xlet.XletContext c) {
        if (c == null) return "0/0";
        return hex(c.getXletProperty("dvb.org.id")) + "/" + hex(c.getXletProperty("dvb.app.id"));
    }
    public static synchronized void bind(javax.tv.xlet.XletContext c, String n, java.rmi.Remote o) throws java.rmi.AlreadyBoundException {
        if (n == null || o == null) throw new NullPointerException();
        String k = owner(c) + "/" + n;
        if (reg.containsKey(k)) throw new java.rmi.AlreadyBoundException(n);
        reg.put(k, o);
    }
    public static synchronized void rebind(javax.tv.xlet.XletContext c, String n, java.rmi.Remote o) {
        if (n == null || o == null) throw new NullPointerException();
        reg.put(owner(c) + "/" + n, o);
    }
    public static synchronized void unbind(javax.tv.xlet.XletContext c, String n) throws java.rmi.NotBoundException {
        if (reg.remove(owner(c) + "/" + n) == null) throw new java.rmi.NotBoundException(n);
    }
    public static synchronized java.rmi.Remote lookup(javax.tv.xlet.XletContext c, String path) throws java.rmi.NotBoundException {
        if (path == null) throw new NullPointerException();
        String p = path;
        while (p.startsWith("/")) p = p.substring(1);
        String[] parts = p.split("/", 3);
        java.rmi.Remote r = null;
        if (parts.length == 3) r = reg.get(hex(parts[0]) + "/" + hex(parts[1]) + "/" + parts[2]);
        if (r == null) r = reg.get(owner(c) + "/" + p);
        if (r == null) { // a bare name another application bound
            for (Map.Entry<String, java.rmi.Remote> e : reg.entrySet()) {
                String k = e.getKey();
                int i = k.indexOf('/', k.indexOf('/') + 1);
                if (k.substring(i + 1).equals(p)) {
                    r = e.getValue();
                    break;
                }
            }
        }
        if (r == null) throw new java.rmi.NotBoundException(path);
        return r;
    }
    public static synchronized String[] list(javax.tv.xlet.XletContext c) {
        String mine = owner(c) + "/";
        List<String> o = new ArrayList<String>();
        for (String k : reg.keySet()) if (k.startsWith(mine)) o.add(k.substring(mine.length()));
        return o.toArray(new String[o.size()]);
    }
}
