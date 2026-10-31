package org.havi.ui;
public class HFlatEffectMatte implements HMatte, HAnimateEffect {
    private volatile float[] data;
    private final HEffect effect = new HEffect() {
        int frames() { float[] d = data; return d == null ? 0 : d.length; }
        void shown() { witty.bdj.Plane.invalidate(); }
    };
    public HFlatEffectMatte() {}
    public HFlatEffectMatte(float[] d) { data = d; }
    public void setMatteData(float[] d) { data = d; effect.position(0); }
    public float[] getMatteData() { return data; }
    final float now() { float[] d = data; int p = effect.position(); return d == null || p >= d.length ? 1 : d[p]; }
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
