package javax.media;
public interface GainControl extends Control {
    void setMute(boolean m);
    boolean getMute();
    float setDB(float g);
    float getDB();
    float setLevel(float l);
    float getLevel();
    void addGainChangeListener(GainChangeListener l);
    void removeGainChangeListener(GainChangeListener l);
}
