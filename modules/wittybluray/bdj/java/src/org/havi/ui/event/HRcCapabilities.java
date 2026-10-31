package org.havi.ui.event;
public class HRcCapabilities extends HKeyCapabilities {
    protected HRcCapabilities() {}
    public static boolean getInputDeviceSupported() { return true; }
    public static boolean isSupported(int code) { return known(code); }
    public static HEventRepresentation getRepresentation(int code) { return known(code) ? new HEventRepresentation(code) : null; }
}
