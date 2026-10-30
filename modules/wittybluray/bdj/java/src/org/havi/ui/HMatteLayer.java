package org.havi.ui;
public interface HMatteLayer {
    void setMatte(HMatte m) throws HMatteException;
    HMatte getMatte();
}
