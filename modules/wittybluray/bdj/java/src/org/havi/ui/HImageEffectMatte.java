package org.havi.ui;
public class HImageEffectMatte implements HMatte, HAnimateEffect {
    private volatile java.awt.Image[] data;
    private final java.util.Map<Integer, java.awt.Point> offsets = new java.util.concurrent.ConcurrentHashMap<Integer, java.awt.Point>();
    private final HEffect effect = new HEffect() {
        int frames() { java.awt.Image[] d = data; return d == null ? 0 : d.length; }
        void shown() { witty.bdj.Plane.invalidate(); }
    };
    public HImageEffectMatte() {}
    public HImageEffectMatte(java.awt.Image[] d) { data = d; }
    public void setMatteData(java.awt.Image[] d) { data = d; offsets.clear(); effect.position(0); }
    public java.awt.Image[] getMatteData() { return data; }
    public void setOffset(java.awt.Point p, int i) { if (p == null) offsets.remove(i); else offsets.put(i, p); witty.bdj.Plane.invalidate(); }
    public java.awt.Point getOffset(int i) { java.awt.Point p = offsets.get(i); return p != null ? p : new java.awt.Point(0, 0); }
    public void start() { effect.start(); }
    public void stop() { effect.stop(); }
    public boolean isAnimated() { return effect.running(); }
    public void setPosition(int p) { effect.position(p); }
    public int getPosition() { return effect.position(); }
    public void setRepeatCount(int c) { effect.repeat(c); }
    public int getRepeatCount() { return effect.repeat(); }
    public void setDelay(int d) { effect.delay(d); }
    public int getDelay() { return effect.delay(); }
    public void setPlayMode(int m) { effect.mode(m); }
    public int getPlayMode() { return effect.mode(); }
}
