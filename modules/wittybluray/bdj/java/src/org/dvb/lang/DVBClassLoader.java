package org.dvb.lang;

import witty.shim.Loader;
import witty.shim.URL;

/**
 * A class loader over more of the disc's JAR files. Their classes join the one
 * loader all of a disc's code has, under the same rules; a URL that does not name
 * the disc's own files adds nothing, as if the file were missing.
 */
public abstract class DVBClassLoader extends Loader {
    protected DVBClassLoader(URL[] urls) { this(urls, null); }
    protected DVBClassLoader(URL[] urls, Loader parent) {
        super(parent);
        if (urls == null) return;
        for (int i = 0; i < urls.length; i++) {
            String p = urls[i] == null ? null : path(urls[i].toExternalForm());
            if (p == null) continue;
            try {
                addDiscFile(p);
            } catch (java.io.IOException e) {
                // Not the disc's own: its classes will not be found.
            }
        }
    }
    private static String path(String u) {
        if (!u.startsWith("file:")) return null;
        String p = u.substring(5);
        while (p.startsWith("//")) p = p.substring(1);
        if (p.length() > 2 && p.charAt(0) == '/' && p.charAt(2) == ':') p = p.substring(1);
        return p;
    }
    public static DVBClassLoader newInstance(URL[] urls) { return new DVBClassLoader(urls) {}; }
    public static DVBClassLoader newInstance(URL[] urls, Loader parent) { return new DVBClassLoader(urls, parent) {}; }
}
