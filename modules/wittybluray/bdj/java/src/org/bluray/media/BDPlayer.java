package org.bluray.media;

import java.awt.Component;
import java.util.List;
import javax.media.*;
import witty.bdj.Host;
import witty.bdj.Playback;

/**
 * A JMF player over the server's video: bd:// locators name playlists; bd://SOUND: names button sounds.
 * Listeners hear on the event queue, so that disc code never runs on the threads the platform reads the link with.
 */
public class BDPlayer implements Player, Playback.Listener, PlaybackControl, PlayListChangeControl, AngleControl,
        BackgroundVideoPresentationControl, javax.tv.media.AWTVideoSizeControl, UOMaskTableControl {
    public static final TimeBase SYSTEM = new TimeBase() {
        public Time getTime() { return new Time(getNanoseconds()); }
        public long getNanoseconds() { return java.lang.System.nanoTime(); }
    };
    private final List<ControllerListener> listeners = new java.util.concurrent.CopyOnWriteArrayList<ControllerListener>();
    private final List<PlaybackListener> pbs = new java.util.concurrent.CopyOnWriteArrayList<PlaybackListener>();
    private final List<PlayListChangeListener> plc = new java.util.concurrent.CopyOnWriteArrayList<PlayListChangeListener>();
    private final List<AngleChangeListener> angle = new java.util.concurrent.CopyOnWriteArrayList<AngleChangeListener>();
    private final List<UOMaskTableListener> uos = new java.util.concurrent.CopyOnWriteArrayList<UOMaskTableListener>();
    private volatile int state = Unrealized, target = Unrealized;
    private org.bluray.net.BDLocator loc;
    private int soundId = -1;
    private org.dvb.media.VideoTransformation vt = new org.dvb.media.VideoTransformation();
    private final Component visual = new Component() { public boolean isOpaque() { return false; } };
    private final OverallGain overall = new OverallGain();
    private final Control[] controls = {this, new PrimaryAudio(), new Subtitles(), new SecondaryAudio(), overall, new PrimaryGain(), new SecondaryGain(), new Panning(), new NoPiP()};

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

    private static void later(final Runnable r) {
        java.awt.EventQueue.invokeLater(new Runnable() {
            public void run() {
                try {
                    r.run();
                } catch (Throwable t) {
                    Host.log("player listener: " + t);
                }
            }
        });
    }
    private void post(final ControllerEvent e) {
        later(new Runnable() {
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
            final PlaybackMarkEvent e = new PlaybackMarkEvent(this, Integer.parseInt(w[1]));
            later(new Runnable() { public void run() { for (PlaybackListener l : pbs) l.markReached(e); } });
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
    public Control[] getControls() { return (Control[]) controls.clone(); }
    /** Only ever looks at platform classes, and without initialising them: the name comes from the disc. */
    public Control getControl(String type) {
        if (type == null) return null;
        Class<?> c;
        try {
            c = Class.forName(type, false, BDPlayer.class.getClassLoader());
        } catch (ClassNotFoundException e) {
            return null;
        } catch (LinkageError e) {
            return null;
        }
        for (int i = 0; i < controls.length; i++) if (c.isInstance(controls[i])) return controls[i];
        return null;
    }
    public void addControllerListener(ControllerListener l) { if (l != null) listeners.add(l); }
    public void removeControllerListener(ControllerListener l) { listeners.remove(l); }
    public Component getVisualComponent() { return visual; }
    public GainControl getGainControl() { return overall; }
    public Component getControlPanelComponent() { return null; }
    public Component getControlComponent() { return null; }
    public void addController(Controller c) {}
    public void removeController(Controller c) {}
    public void setSource(javax.media.protocol.DataSource s) {}
    // Playback.
    public void addPlaybackControlListener(PlaybackListener l) { if (l != null) pbs.add(l); }
    public void removePlaybackControlListener(PlaybackListener l) { pbs.remove(l); }
    public void skipToMark(int m) { double[] a = Playback.marks; if (m < 0 || m >= a.length) throw new IllegalArgumentException(); Playback.seek(a[m]); }
    public boolean skipToNextMark(int type) { double n = Playback.now(); for (int i = 0; i < Playback.marks.length; i++) if (Playback.marks[i] > n + 0.1) { Playback.seek(Playback.marks[i]); return true; } return false; }
    public boolean skipToPreviousMark(int type) { double n = Playback.now(); for (int i = Playback.marks.length - 1; i >= 0; i--) if (Playback.marks[i] < n - 1) { Playback.seek(Playback.marks[i]); return true; } return false; }
    public void skipToPlayItem(int i) { double[] a = Playback.items; if (i < 0 || i >= a.length) throw new IllegalArgumentException(); Playback.seek(a[i]); }
    // Playlists.
    public void selectPlayList(org.bluray.net.BDLocator l) throws InvalidPlayListException {
        if (l == null || l.getPlayListId() < 0) throw new InvalidPlayListException(String.valueOf(l));
        loc = l;
        Playback.addListener(this);
        Playback.play(l.getPlayListId(), l.getPlayItemId(), l.getMarkId(), 0);
        state = Started;
        target = Started;
        final PlayListChangeEvent e = new PlayListChangeEvent(this);
        later(new Runnable() { public void run() { for (PlayListChangeListener p : plc) p.playListChange(e); } });
    }
    public org.bluray.net.BDLocator getCurrentPlayList() { return loc; }
    public void addPlayListChangeListener(PlayListChangeListener l) { if (l != null) plc.add(l); }
    public void removePlayListChangeListener(PlayListChangeListener l) { plc.remove(l); }
    // Angles: player status registers the server acts on when they change.
    public int getCurrentAngle() { return Host.PSR[3]; }
    public void selectDefaultAngle() { Host.setPSR(3, 1); }
    public void selectAngle(int a) throws InvalidAngleException {
        if (a < 1 || a > 9) throw new InvalidAngleException(String.valueOf(a));
        Host.setPSR(3, a);
        final AngleChangeEvent e = new AngleChangeEvent(this, a);
        later(new Runnable() { public void run() { for (AngleChangeListener l : angle) l.angleChange(e); } });
    }
    public void addAngleChangeListener(AngleChangeListener l) { if (l != null) angle.add(l); }
    public void removeAngleChangeListener(AngleChangeListener l) { angle.remove(l); }
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
    public void addUOMaskTableEventListener(UOMaskTableListener l) { if (l != null) uos.add(l); }
    public void removeUOMaskTableEventListener(UOMaskTableListener l) { uos.remove(l); }

    private abstract static class Plain implements Control {
        public Component getControlComponent() { return null; }
    }
    /** A stream chosen by a player status register; the server acts on the register when it changes. */
    private abstract static class Stream extends Plain implements StreamControl {
        private final int psr, mask;
        Stream(int r, int m) { psr = r; mask = m; }
        public int getCurrentStreamNumber() { return Host.PSR[psr] & mask; }
        public void selectStreamNumber(int n) throws StreamNotAvailableException {
            if (n <= 0 || n > mask) throw new StreamNotAvailableException(String.valueOf(n));
            Host.setPSR(psr, (Host.PSR[psr] & ~mask) | n);
        }
        public int[] listAvailableStreamNumbers() { return new int[0]; }
        public String getCurrentLanguage() { return "und"; }
        public String[] listAvailableLanguages() { return new String[0]; }
        public String selectDefaultLanguage() { return "und"; }
        public void selectLanguage(String l) {}
    }
    private static final class PrimaryAudio extends Stream implements PrimaryAudioControl { PrimaryAudio() { super(1, 0xff); } }
    private static final class SecondaryAudio extends Stream implements SecondaryAudioControl { SecondaryAudio() { super(14, 0xff); } }
    private static final class Subtitles extends Stream implements SubtitlingControl {
        Subtitles() { super(2, 0xfff); }
        public boolean setSubtitling(boolean on) { int v = Host.PSR[2]; Host.setPSR(2, on ? v | 0x80000000 : v & 0x7fffffff); return on; }
        public boolean isSubtitlingOn() { return Host.PSR[2] < 0; }
        public boolean isSubtitlingAvailable() { return true; }
    }
    /** Sound level: the viewer has the volume; a disc's own level is remembered and not applied. */
    private abstract static class Gain extends Plain implements GainControl {
        private final List<GainChangeListener> ls = new java.util.concurrent.CopyOnWriteArrayList<GainChangeListener>();
        private volatile boolean mute;
        private volatile float db, level = 1;
        public void setMute(boolean m) { mute = m; changed(); }
        public boolean getMute() { return mute; }
        public float setDB(float g) { db = g; changed(); return g; }
        public float getDB() { return db; }
        public float setLevel(float l) { level = Math.max(0f, Math.min(1f, l)); changed(); return level; }
        public float getLevel() { return level; }
        public void addGainChangeListener(GainChangeListener l) { if (l != null) ls.add(l); }
        public void removeGainChangeListener(GainChangeListener l) { ls.remove(l); }
        private void changed() {
            if (ls.isEmpty()) return;
            final GainChangeEvent e = new GainChangeEvent(this, mute, db, level);
            later(new Runnable() { public void run() { for (GainChangeListener l : ls) l.gainChange(e); } });
        }
    }
    private static final class OverallGain extends Gain implements OverallGainControl {}
    private static final class PrimaryGain extends Gain implements PrimaryGainControl {}
    private static final class SecondaryGain extends Gain implements SecondaryGainControl {}
    private static final class Panning extends Plain implements PanningControl {
        private final List<PanningChangeListener> ls = new java.util.concurrent.CopyOnWriteArrayList<PanningChangeListener>();
        private volatile float x, y;
        public void setLeftRight(float v) { x = Math.max(-1f, Math.min(1f, v)); changed(); }
        public float getLeftRight() { return x; }
        public void setFrontRear(float v) { y = Math.max(-1f, Math.min(1f, v)); changed(); }
        public float getFrontRear() { return y; }
        public void addPanningChangeListener(PanningChangeListener l) { if (l != null) ls.add(l); }
        public void removePanningChangeListener(PanningChangeListener l) { ls.remove(l); }
        private void changed() {
            if (ls.isEmpty()) return;
            final PanningChangeEvent e = new PanningChangeEvent(this, x, y);
            later(new Runnable() { public void run() { for (PanningChangeListener l : ls) l.panningChange(e); } });
        }
    }
    /** One video stream, so a picture-in-picture is never available and is never on screen. */
    private static final class NoPiP extends Plain implements AsynchronousPiPControl {
        public void setDisplay(boolean on) {}
        public boolean getIsDisplay() { return false; }
        public void setFullScreen(boolean on) {}
        public boolean getFullScreen() { return false; }
        public boolean isSynchronized() { return false; }
        public void addPiPStatusListener(PiPStatusListener l) {}
        public void removePiPStatusListener(PiPStatusListener l) {}
        public void start() {}
        public void stop() {}
        public void pause() {}
        public void resume() {}
    }
}
