package org.dvb.io.ixc;

import java.util.HashMap;
import java.util.Map;

/** Applications on one disc sharing objects: a registry they all see. */
public class IxcRegistry {
    private static final Map<String, Object> reg = new HashMap<String, Object>();
    public static synchronized void bind(javax.tv.xlet.XletContext c, String n, java.rmi.Remote o) throws java.rmi.AlreadyBoundException { if (reg.containsKey(n)) throw new java.rmi.AlreadyBoundException(n); reg.put(n, o); }
    public static synchronized void rebind(javax.tv.xlet.XletContext c, String n, java.rmi.Remote o) { reg.put(n, o); }
    public static synchronized void unbind(javax.tv.xlet.XletContext c, String n) throws java.rmi.NotBoundException { if (reg.remove(n) == null) throw new java.rmi.NotBoundException(n); }
    public static synchronized java.rmi.Remote lookup(javax.tv.xlet.XletContext c, String n) throws java.rmi.NotBoundException { Object o = reg.get(n); if (o == null) throw new java.rmi.NotBoundException(n); return (java.rmi.Remote) o; }
    public static synchronized String[] list(javax.tv.xlet.XletContext c) { return reg.keySet().toArray(new String[0]); }
}
