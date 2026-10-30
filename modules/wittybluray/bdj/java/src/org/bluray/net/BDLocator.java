package org.bluray.net;

import org.davic.net.InvalidLocatorException;

/** bd://[disc-id.]title[.PLAYLIST:n[.ITEM:n][.MARK:n]][.A1:n]...; bd://SOUND:id; bd://JAR:name */
public class BDLocator extends org.davic.net.Locator {
    public static final int NOTIFICATION_ON_ENTRY = 1, NOTIFICATION_ON_EXIT = 2;
    private int title = -1, playList = -1, item = -1, mark = -1, sound = -1, a1 = -1, a2 = -1, pg = -1, v2 = -1, pip = -1, ig = -1;
    private String disc = "", jar = "";
    public BDLocator(String s) throws InvalidLocatorException {
        super(s);
        if (s == null || !s.startsWith("bd://")) throw new InvalidLocatorException(String.valueOf(s));
        String[] parts = s.substring(5).split("\\.");
        try {
            for (int i = 0; i < parts.length; i++) {
                String p = parts[i];
                int c = p.indexOf(':');
                if (c < 0) {
                    if (p.length() == 32) disc = p;
                    else if (p.length() > 0) title = Integer.parseInt(p, 16);
                    continue;
                }
                String k = p.substring(0, c), v = p.substring(c + 1);
                if (k.equals("PLAYLIST")) playList = Integer.parseInt(v);
                else if (k.equals("ITEM")) item = Integer.parseInt(v, 16);
                else if (k.equals("MARK")) mark = Integer.parseInt(v, 16);
                else if (k.equals("SOUND")) sound = Integer.parseInt(v, 16);
                else if (k.equals("JAR")) jar = v;
                else if (k.equals("A1")) a1 = Integer.parseInt(v, 16);
                else if (k.equals("A2")) a2 = Integer.parseInt(v, 16);
                else if (k.equals("P")) pg = Integer.parseInt(v, 16);
                else if (k.equals("V2")) v2 = Integer.parseInt(v, 16);
                else if (k.equals("PP")) pip = Integer.parseInt(v, 16);
            }
        } catch (NumberFormatException e) {
            throw new InvalidLocatorException(s);
        }
    }
    public BDLocator(String disc, int title, int playList) throws InvalidLocatorException { this(make(disc, title, playList, -1, -1)); }
    public BDLocator(String disc, int title, int playList, int item, int mark, String[] comps) throws InvalidLocatorException { this(make(disc, title, playList, item, mark)); }
    private static String make(String d, int t, int p, int i, int m) {
        StringBuilder s = new StringBuilder("bd://");
        if (d != null && d.length() > 0) s.append(d).append('.');
        if (t >= 0) s.append(Integer.toHexString(t));
        if (p >= 0) s.append(t >= 0 ? "." : "").append("PLAYLIST:").append(String.format("%05d", p));
        if (i >= 0) s.append(".ITEM:").append(Integer.toHexString(i));
        if (m >= 0) s.append(".MARK:").append(Integer.toHexString(m));
        return s.toString();
    }
    public String getPathSegments() { return url.substring(5); }
    public String getDiscId() { return disc; }
    public int getTitleNumber() { return title; }
    public int getPlayListId() { return playList; }
    public int getPlayItemId() { return item; }
    public int getMarkId() { return mark; }
    public int getSoundId() { return sound; }
    public String getJarFileName() { return jar; }
    public int getPrimaryAudioStreamNumber() { return a1; }
    public int getSecondaryAudioStreamNumber() { return a2; }
    public int getPGTextStreamNumber() { return pg; }
    public int getSecondaryVideoStreamNumber() { return v2; }
    public int getPiPPGStreamNumber() { return pip; }
    public int getIGStreamNumber() { return ig; }
    public int getComponentTagsCount() { return 0; }
    public String[] getComponentTags() { return new String[0]; }
    public boolean equals(Object o) { return o instanceof BDLocator && url.equals(((BDLocator) o).url); }
    public int hashCode() { return url.hashCode(); }
}
