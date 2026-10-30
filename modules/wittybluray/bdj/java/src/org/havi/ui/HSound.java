package org.havi.ui;

import java.net.URL;
import witty.bdj.Host;

/** A short sound; the viewer's browser plays it. */
public class HSound {
    private int id = -1;
    public void load(String location) { id = soundId(location); }
    public void load(URL u) { id = soundId(String.valueOf(u)); }
    public void load(witty.shim.URL u) { id = soundId(String.valueOf(u)); }
    public void set(byte[] data) {}
    public void play() { if (id >= 0) Host.send("sound " + id); }
    public void stop() {}
    public void loop() { play(); }
    public void dispose() {}
    public void addHSoundListener(Object l) {}
    public void removeHSoundListener(Object l) {}
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
