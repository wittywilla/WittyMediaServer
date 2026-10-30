package javax.media;
public class GainChangeEvent extends java.util.EventObject {
    private final boolean mute;
    private final float db, level;
    public GainChangeEvent(GainControl s, boolean m, float d, float l) { super(s); mute = m; db = d; level = l; }
    public boolean getMute() { return mute; }
    public float getDB() { return db; }
    public float getLevel() { return level; }
}
