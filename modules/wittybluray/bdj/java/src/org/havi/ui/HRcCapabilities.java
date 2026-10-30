package org.havi.ui;
public class HRcCapabilities extends HKeyCapabilities {
    public static boolean isSupported(int code) { return true; }
    public static HEventRepresentation getRepresentation(int code) { return new HEventRepresentation(); }
    public static boolean getInputDeviceSupported() { return true; }
}
