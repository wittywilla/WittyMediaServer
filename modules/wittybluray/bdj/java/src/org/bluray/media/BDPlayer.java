package org.bluray.media;

import java.awt.Component;
import java.util.ArrayList;
import java.util.List;
import javax.media.*;
import witty.bdj.Host;
import witty.bdj.Playback;

/** A JMF player over the server's video: bd:// locators name playlists; bd://SOUND: names button sounds. */
public class BDPlayer implements Player, Playback.Listener, PlaybackControl, PlayListChangeControl, AngleControl, PrimaryAudioControl, SubtitlingControl,
        OverallGainControl, BackgroundVideoPresentationControl, javax.tv.media.AWTVideoSizeControl, UOMaskTableControl {
    public static final TimeBase SYSTEM = new TimeBase() {
        public Time getTime() { return new Time(getNanoseconds()); }
        public long getNanoseconds() { return java.lang.System.nanoTime(); }
    };
    private final List<ControllerListener> listeners = new java.util.concurrent.CopyOnWriteArrayList<ControllerListener>();
    private final List<PlaybackListener> pbs = new java.util.concurrent.CopyOnWriteArrayList<PlaybackListener>();
    private final List<PlayListChangeListener> plc = new java.util.concurrent.CopyOnWriteArrayList<PlayListChangeListener>();
    private final List<AngleChangeListener> angle = new java.util.concurrent.CopyOnWriteArrayList<AngleChangeListener>();
    private volatile int state = Unrealized, target = Unrealized;
    private org.bluray.net.BDLocator loc;
    private int soundId = -1;
    private float gain = 0, level = 1;
    private boolean mute;
    private org.dvb.media.VideoTransformation vt = new org.dvb.media.VideoTransformation();
    private final Component visual = new Component() { public boolean isOpaque() { return false; } };

    public static Player create(String l) throws NoPlayerException {
        BDPlayer p = new BDPlayer();
        try {
            p.loc = new org.bluray.net.BDLocator(l);
            if (l.indexOf("SOUND:") >= 0) p.soundId = p.loc.getSoundId();
        } catch (org.davic.net.InvalidLocatorException e) {
            throw new NoPlayerException(l);
        }
        return p;
    }

    private void post(final ControllerEvent e) {
        java.awt.EventQueue.invokeLater(new Runnable() {
            public void run() {
                for (ControllerListener l : listeners) {
                    try {
                        l.controllerUpdate(e);
                    } catch (Throwable t) {
                        Host.log("controller listener: " + t);
                    }
                }
            }
        });
    }
    private void move(int s, ControllerEvent e) { state = s; target = Math.max(target, s); post(e); }

    public int getState() { return state; }
    public int getTargetState() { return target; }
    public void realize() { if (state < Realized) { int p = state; target = Realized; move(Realized, new RealizeCompleteEvent(this, p, Realized, Realized)); } }
    public void prefetch() { realize(); if (state < Prefetched) { int p = state; target = Prefetched; move(Prefetched, new PrefetchCompleteEvent(this, p, Prefetched, Prefetched)); } }
    public void start() { prefetch(); syncStart(new Time(0L)); }
    public void syncStart(Time t) {
        if (state == Started) return;
        int p = state;
        state = Started;
        target = Started;
        if (soundId >= 0) {
            Host.send("sound " + soundId);
        } else {
            Playback.addListener(this);
            if (Playback.playlist == loc.getPlayListId() && loc.getPlayListId() >= 0 && Playback.isRunning() == false && Playback.now() > 0) Playback.resume();
            else Playback.play(loc.getPlayListId(), loc.getPlayItemId(), loc.getMarkId(), 0);
        }
        post(new StartEvent(this, p, Started, Started, getMediaTime(), SYSTEM.getTime()));
    }
    public void stop() {
        if (state != Started) return;
        if (soundId < 0) Playback.pause();
        state = Prefetched;
        target = Prefetched;
        post(new StopByRequestEvent(this, Started, Prefetched, Prefetched, getMediaTime()));
    }
    public void deallocate() { if (state == Started) stop(); state = Realized; target = Realized; post(new DeallocateEvent(this, Prefetched, Realized, Realized, getMediaTime())); }
    public void close() {
        if (state == Started && soundId < 0) Playback.stop();
        state = Unrealized;
        Playback.removeListener(this);
        post(new ControllerClosedEvent(this));
    }
    public void playbackEvent(String what, String[] w) {
        if (what.equals("ended")) {
            state = Prefetched;
            target = Prefetched;
            post(new EndOfMediaEvent(this, Started, Prefetched, Prefetched, getMediaTime()));
        } else if (what.equals("mark")) {
            PlaybackMarkEvent e = new PlaybackMarkEvent(this, Integer.parseInt(w[1]));
            for (PlaybackListener l : pbs) l.markReached(e);
        } else if (what.equals("failed")) {
            post(new ResourceUnavailableEvent(this, w.length > 1 ? w[1] : ""));
        }
    }
    public void setTimeBase(TimeBase b) {}
    public TimeBase getTimeBase() { return SYSTEM; }
    public void setStopTime(Time t) {}
    public Time getStopTime() { return Clock.RESET; }
    public void setMediaTime(Time t) { if (soundId < 0) Playback.seek(t.getSeconds()); post(new MediaTimeSetEvent(this, t)); }
    public Time getMediaTime() { return new Time(Playback.now()); }
    public long getMediaNanoseconds() { return (long) (Playback.now() * 1e9); }
    public Time getSyncTime() { return getMediaTime(); }
    public Time mapToTimeBase(Time t) { return t; }
    public float getRate() { return (float) Playback.getRate(); }
    public float setRate(float r) { Playback.setRate(r); post(new RateChangeEvent(this, r)); return r; }
    public Time getDuration() { return Playback.duration > 0 ? new Time(Playback.duration) : Duration.DURATION_UNKNOWN; }
    public Time getStartLatency() { return LATENCY_UNKNOWN; }
    public Control[] getControls() { return new Control[] {this}; }
    public Control getControl(String type) {
        try {
            return Class.forName(type).isInstance(this) ? this : null;
        } catch (ClassNotFoundException e) {
            return null;
        }
    }
    public void addControllerListener(ControllerListener l) { listeners.add(l); }
    public void removeControllerListener(ControllerListener l) { listeners.remove(l); }
    public Component getVisualComponent() { return visual; }
    public GainControl getGainControl() { return this; }
    public Component getControlPanelComponent() { return null; }
    public Component getControlComponent() { return null; }
    public void addController(Controller c) {}
    public void removeController(Controller c) {}
    public void setSource(javax.media.protocol.DataSource s) {}
    // Playback.
    public void addPlaybackControlListener(PlaybackListener l) { pbs.add(l); }
    public void removePlaybackControlListener(PlaybackListener l) { pbs.remove(l); }
    public void skipToMark(int m) { double[] a = Playback.marks; if (m < 0 || m >= a.length) throw new IllegalArgumentException(); Playback.seek(a[m]); }
    public boolean skipToNextMark(int type) { double n = Playback.now(); for (int i = 0; i < Playback.marks.length; i++) if (Playback.marks[i] > n + 0.1) { Playback.seek(Playback.marks[i]); return true; } return false; }
    public boolean skipToPreviousMark(int type) { double n = Playback.now(); for (int i = Playback.marks.length - 1; i >= 0; i--) if (Playback.marks[i] < n - 1) { Playback.seek(Playback.marks[i]); return true; } return false; }
    public void skipToPlayItem(int i) { double[] a = Playback.items; if (i < 0 || i >= a.length) throw new IllegalArgumentException(); Playback.seek(a[i]); }
    // Playlists.
    public void selectPlayList(org.bluray.net.BDLocator l) {
        loc = l;
        Playback.addListener(this);
        Playback.play(l.getPlayListId(), l.getPlayItemId(), l.getMarkId(), 0);
        state = Started;
        target = Started;
        PlayListChangeEvent e = new PlayListChangeEvent(this);
        for (PlayListChangeListener p : plc) p.playListChange(e);
    }
    public org.bluray.net.BDLocator getCurrentPlayList() { return loc; }
    public void addPlayListChangeListener(PlayListChangeListener l) { plc.add(l); }
    public void removePlayListChangeListener(PlayListChangeListener l) { plc.remove(l); }
    // Angles: player status registers the server acts on when they change.
    public int getCurrentAngle() { return Host.PSR[3]; }
    public void selectDefaultAngle() { Host.setPSR(3, 1); }
    public void selectAngle(int a) { Host.setPSR(3, a); AngleChangeEvent e = new AngleChangeEvent(this, a); for (AngleChangeListener l : angle) l.angleChange(e); }
    public void addAngleChangeListener(AngleChangeListener l) { angle.add(l); }
    public void removeAngleChangeListener(AngleChangeListener l) { angle.remove(l); }
    // Streams.
    public int getCurrentStreamNumber() { return Host.PSR[1]; }
    public void selectStreamNumber(int n) { Host.setPSR(1, n); }
    public int[] listAvailableStreamNumbers() { return new int[0]; }
    public String getCurrentLanguage() { return "und"; }
    public String[] listAvailableLanguages() { return new String[0]; }
    public String selectDefaultLanguage() { return "und"; }
    public void selectLanguage(String l) {}
    public boolean setSubtitling(boolean on) { int v = Host.PSR[2]; Host.setPSR(2, on ? v | 0x80000000 : v & 0x7fffffff); return on; }
    public boolean isSubtitlingOn() { return Host.PSR[2] < 0; }
    public boolean isSubtitlingAvailable() { return true; }
    // Sound level: the viewer has the volume; a disc's own level is not applied.
    public void setMute(boolean m) { mute = m; }
    public boolean getMute() { return mute; }
    public float setDB(float g) { gain = g; return g; }
    public float getDB() { return gain; }
    public float setLevel(float l) { level = l; return l; }
    public float getLevel() { return level; }
    public void addGainChangeListener(GainChangeListener l) {}
    public void removeGainChangeListener(GainChangeListener l) {}
    // Video placement.
    private org.havi.ui.HScreenRectangle whole() { return new org.havi.ui.HScreenRectangle(0, 0, 1, 1); }
    public java.awt.Dimension getInputVideoSize() { return new java.awt.Dimension(Host.width, Host.height); }
    public java.awt.Dimension getVideoSize() { return new java.awt.Dimension(Host.width, Host.height); }
    public org.havi.ui.HScreenRectangle getActiveVideoArea() { return whole(); }
    public org.havi.ui.HScreenRectangle getActiveVideoAreaOnScreen() { return whole(); }
    public org.havi.ui.HScreenRectangle getTotalVideoArea() { return whole(); }
    public org.havi.ui.HScreenRectangle getTotalVideoAreaOnScreen() { return whole(); }
    public boolean supportsClipping() { return false; }
    public java.awt.Rectangle setClipRegion(java.awt.Rectangle r) { return new java.awt.Rectangle(0, 0, Host.width, Host.height); }
    public java.awt.Rectangle getClipRegion() { return new java.awt.Rectangle(0, 0, Host.width, Host.height); }
    public float[] supportsArbitraryHorizontalScaling() { return new float[] {0.25f, 1f}; }
    public float[] supportsArbitraryVerticalScaling() { return new float[] {0.25f, 1f}; }
    public float[] getHorizontalScalingFactors() { return new float[] {1f, 0.5f, 0.25f}; }
    public float[] getVerticalScalingFactors() { return new float[] {1f, 0.5f, 0.25f}; }
    public byte getPositioningCapability() { return 1; }
    public boolean setVideoTransformation(org.dvb.media.VideoTransformation t) {
        vt = t;
        float[] s = t.getScalingFactors();
        org.havi.ui.HScreenPoint p = t.getVideoPosition();
        Playback.videoRect((int) (p.x * Host.width), (int) (p.y * Host.height), (int) (s[0] * Host.width), (int) (s[1] * Host.height));
        return true;
    }
    public org.dvb.media.VideoTransformation getVideoTransformation() { return vt; }
    public org.dvb.media.VideoTransformation getClosestMatch(org.dvb.media.VideoTransformation t) { return t; }
    public javax.tv.media.AWTVideoSize getSize() { java.awt.Rectangle r = new java.awt.Rectangle(0, 0, Host.width, Host.height); return new javax.tv.media.AWTVideoSize(r, r); }
    public javax.tv.media.AWTVideoSize getDefaultSize() { return getSize(); }
    public java.awt.Dimension getSourceVideoSize() { return new java.awt.Dimension(Host.width, Host.height); }
    public boolean setSize(javax.tv.media.AWTVideoSize s) { java.awt.Rectangle d = s.getDestination(); Playback.videoRect(d.x, d.y, d.width, d.height); return true; }
    public javax.tv.media.AWTVideoSize checkSize(javax.tv.media.AWTVideoSize s) { return s; }
    public boolean[] getMaskedUOTable() { return new boolean[64]; }
    public void addUOMaskTableChangeListener(Object l) {}
    public void removeUOMaskTableChangeListener(Object l) {}
}
