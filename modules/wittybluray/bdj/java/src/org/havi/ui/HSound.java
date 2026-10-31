package org.havi.ui;

import witty.bdj.Host;

/** A short sound; the viewer's browser plays it. Only the disc's own sounds, named bd://SOUND:id, can be played. */
public class HSound {
    private volatile int id = -1;
    public HSound() {}
    public void load(String location) throws java.io.IOException, SecurityException { id = soundId(location); }
    public void load(witty.shim.URL u) throws java.io.IOException, SecurityException { id = soundId(String.valueOf(u)); }
    public void set(byte[] data) {}
    public void play() { if (id >= 0) Host.send("sound " + id); }
    public void stop() {}
    public void loop() { play(); }
    public void dispose() { id = -1; }
    private static int soundId(String s) {
        int i = s == null ? -1 : s.lastIndexOf("SOUND:");
        if (i < 0) return -1;
        try {
            return Integer.parseInt(s.substring(i + 6).trim(), 16);
        } catch (NumberFormatException e) {
            return -1;
        }
    }
}
