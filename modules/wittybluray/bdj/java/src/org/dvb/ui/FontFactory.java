package org.dvb.ui;

import java.awt.Font;
import java.io.IOException;
import java.util.HashMap;
import java.util.Map;
import witty.bdj.Host;

/** Fonts from the disc's AUXDATA folder, by the names its font index gives them. */
public class FontFactory {
    private static final Map<String, Font> loaded = new HashMap<String, Font>();
    private static Map<String, String> index;
    public FontFactory() throws FontFormatException, IOException {}
    public FontFactory(witty.shim.URL u) throws FontFormatException, IOException {}

    private static synchronized String fileFor(String name) {
        if (index == null) {
            index = new HashMap<String, String>();
            java.io.File f = new java.io.File(new java.io.File(Host.vfsRoot, "BDMV/AUXDATA"), "dvb.fontindex");
            if (f.isFile()) {
                try {
                    String s = new String(java.nio.file.Files.readAllBytes(f.toPath()), "UTF-8");
                    java.util.regex.Matcher m = java.util.regex.Pattern.compile("<name>\\s*([^<]+?)\\s*</name>.*?<filename>\\s*([^<]+?)\\s*</filename>", java.util.regex.Pattern.DOTALL).matcher(s);
                    while (m.find()) index.put(m.group(1), m.group(2));
                } catch (IOException e) {
                    Host.log("font index: " + e);
                }
            }
        }
        return index.get(name);
    }

    public Font createFont(String name, int style, int size) throws FontNotAvailableException, FontFormatException, IOException {
        if (style < 0 || style > 3 || size <= 0) throw new IllegalArgumentException();
        Font base;
        synchronized (loaded) {
            base = loaded.get(name);
            if (base == null) {
                String file = fileFor(name);
                if (file == null) return new Font("SansSerif", style, size);
                java.io.File f = new java.io.File(new java.io.File(Host.vfsRoot, "BDMV/AUXDATA"), new java.io.File(file).getName());
                if (!f.isFile()) throw new FontNotAvailableException(name);
                try {
                    base = Font.createFont(Font.TRUETYPE_FONT, f);
                } catch (java.awt.FontFormatException e) {
                    throw new FontFormatException(name);
                }
                loaded.put(name, base);
            }
        }
        return base.deriveFont(style, (float) size);
    }
}
