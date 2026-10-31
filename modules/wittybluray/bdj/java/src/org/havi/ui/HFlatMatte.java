package org.havi.ui;
public class HFlatMatte implements HMatte {
    private float data = 1;
    public HFlatMatte() {}
    public HFlatMatte(float d) { data = d; }
    public void setMatteData(float d) { data = d; witty.bdj.Plane.invalidate(); }
    public float getMatteData() { return data; }
}
