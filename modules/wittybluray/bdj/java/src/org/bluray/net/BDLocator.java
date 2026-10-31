package org.bluray.net;

import java.util.ArrayList;
import java.util.List;
import org.davic.net.InvalidLocatorException;

/**
 * bd://[disc-id.]title[.PLAYLIST:n[.ITEM:n][.MARK:n]][.A1:n][.A2:n][.V1:n][.V2:n][.P1:n]...;
 * bd://SOUND:id; bd://[disc-id.][title.]JAR:n[/path]. Title and component numbers are hexadecimal; playlists and jars are decimal.
 */
public class BDLocator extends org.davic.net.Locator {
    public static final int NOTIFICATION_ON_ENTRY = 1, NOTIFICATION_ON_EXIT = 2;
    private int title = -1, playList = -1, item = -1, mark = -1, sound = -1, a1 = -1, a2 = -1, v1 = -1, v2 = -1, pg = -1, pip = -1, ig = -1, jarId = -1;
    private String disc = "", jar = "", path = "";
    private final List<String> tags = new ArrayList<String>();
    public BDLocator(String s) throws InvalidLocatorException {
        super(s);
        if (s == null || !s.startsWith("bd://")) throw new InvalidLocatorException(String.valueOf(s));
        String rest = s.substring(5);
        int slash = rest.indexOf('/');
        if (slash >= 0) {
            path = rest.substring(slash + 1);
            rest = rest.substring(0, slash);
        }
        String[] parts = rest.split("\\.");
        try {
            for (int i = 0; i < parts.length; i++) {
                String p = parts[i];
                int c = p.indexOf(':');
                if (c < 0) {
                    if (p.length() == 32) disc = p;
                    else if (p.length() > 0) title = number(p, 16, 0xffff);
                    continue;
                }
                String k = p.substring(0, c), v = p.substring(c + 1);
                if (k.equals("PLAYLIST")) playList = number(v, 10, 99999);
                else if (k.equals("ITEM")) item = number(v, 16, 0xfffff);
                else if (k.equals("MARK")) mark = number(v, 16, 0xfffff);
                else if (k.equals("SOUND")) sound = number(v, 16, 0xff);
                else if (k.equals("JAR")) {
                    jar = v;
                    jarId = number(v, 10, 99999);
                } else {
                    if (k.equals("A1")) a1 = number(v, 16, 0xff);
                    else if (k.equals("A2")) a2 = number(v, 16, 0xff);
                    else if (k.equals("V1")) v1 = number(v, 16, 0xff);
                    else if (k.equals("V2")) v2 = number(v, 16, 0xff);
                    else if (k.equals("P") || k.equals("P1")) pg = number(v, 16, 0xfff);
                    else if (k.equals("PP") || k.equals("P2")) pip = number(v, 16, 0xfff);
                    else if (k.equals("IG")) ig = number(v, 16, 0xff);
                    tags.add(p);
                }
            }
        } catch (NumberFormatException e) {
            throw new InvalidLocatorException(s);
        }
    }
    public BDLocator(String disc, int title, int jar) throws InvalidLocatorException { this(start(disc, title) + (jar >= 0 ? sep(disc, title) + "JAR:" + five(jar) : "")); }
    public BDLocator(String disc, int title, int playList, int item, int mark, String[] comps) throws InvalidLocatorException { this(make(disc, title, playList, item, mark, comps)); }
    private static int number(String v, int radix, int most) {
        int n = Integer.parseInt(v, radix);
        if (n < 0 || n > most) throw new NumberFormatException(v);
        return n;
    }
    private static String five(int n) { StringBuilder s = new StringBuilder(Integer.toString(n)); while (s.length() < 5) s.insert(0, '0'); return s.toString(); }
    private static String fiveHex(int n) { StringBuilder s = new StringBuilder(Integer.toHexString(n)); while (s.length() < 5) s.insert(0, '0'); return s.toString(); }
    private static String start(String d, int t) {
        StringBuilder s = new StringBuilder("bd://");
        boolean hasDisc = d != null && d.length() > 0;
        if (hasDisc) s.append(d);
        if (t >= 0) s.append(hasDisc ? "." : "").append(Integer.toHexString(t));
        return s.toString();
    }
    private static String sep(String d, int t) { return t >= 0 || (d != null && d.length() > 0) ? "." : ""; }
    private static String make(String d, int t, int p, int i, int m, String[] comps) {
        StringBuilder s = new StringBuilder(start(d, t));
        if (p >= 0) s.append(sep(d, t)).append("PLAYLIST:").append(five(p));
        if (i >= 0) s.append(".ITEM:").append(fiveHex(i));
        if (m >= 0) s.append(".MARK:").append(fiveHex(m));
        if (comps != null) for (int c = 0; c < comps.length; c++) if (comps[c] != null) s.append('.').append(comps[c]);
        return s.toString();
    }
    public String getPathSegments() { return url.substring(5); }
    public String getPath() { return path; }
    public String getDiscId() { return disc; }
    public int getTitleNumber() { return title; }
    public int getPlayListId() { return playList; }
    public int getPlayItemId() { return item; }
    public int getMarkId() { return mark; }
    public int getSoundId() { return sound; }
    public int getJar() { return jarId; }
    public String getJarFileName() { return jar; }
    public int getPrimaryAudioStreamNumber() { return a1; }
    public int getSecondaryAudioStreamNumber() { return a2; }
    public int getPrimaryVideoStreamNumber() { return v1; }
    public int getSecondaryVideoStreamNumber() { return v2; }
    public int getPGTextStreamNumber() { return pg; }
    public int getPiPPGStreamNumber() { return pip; }
    public int getIGStreamNumber() { return ig; }
    public int getComponentTagsCount() { return tags.size(); }
    public String[] getComponentTags() { return tags.toArray(new String[tags.size()]); }
    public boolean equals(Object o) { return o instanceof BDLocator && url.equals(((BDLocator) o).url); }
    public int hashCode() { return url.hashCode(); }
}
