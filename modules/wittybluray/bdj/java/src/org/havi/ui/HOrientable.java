package org.havi.ui;
public interface HOrientable {
    int ORIENT_LEFT_TO_RIGHT = 0, ORIENT_RIGHT_TO_LEFT = 1, ORIENT_BOTTOM_TO_TOP = 2, ORIENT_TOP_TO_BOTTOM = 3;
    int getOrientation();
    void setOrientation(int o);
}
