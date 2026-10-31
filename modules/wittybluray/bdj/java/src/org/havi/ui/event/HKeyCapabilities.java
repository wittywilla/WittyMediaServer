package org.havi.ui.event;
import java.awt.event.KeyEvent;
/** The viewer has a remote control's keys, not a keyboard. */
public class HKeyCapabilities {
    protected HKeyCapabilities() {}
    public static boolean getInputDeviceSupported() { return false; }
    public static boolean isSupported(int code) { return known(code); }
    static boolean known(int code) {
        if (code >= KeyEvent.VK_0 && code <= KeyEvent.VK_9) return true;
        switch (code) {
            case KeyEvent.VK_UP: case KeyEvent.VK_DOWN: case KeyEvent.VK_LEFT: case KeyEvent.VK_RIGHT: case KeyEvent.VK_ENTER: case KeyEvent.VK_PAUSE:
            case HRcEvent.VK_COLORED_KEY_0: case HRcEvent.VK_COLORED_KEY_1: case HRcEvent.VK_COLORED_KEY_2: case HRcEvent.VK_COLORED_KEY_3:
            case HRcEvent.VK_REWIND: case HRcEvent.VK_STOP: case HRcEvent.VK_PLAY: case HRcEvent.VK_FAST_FWD: case HRcEvent.VK_TRACK_PREV: case HRcEvent.VK_TRACK_NEXT:
            case HRcEvent.VK_SUBTITLE: case 461: case 462: case 463: case 464: case 465:
                return true;
            default:
                return false;
        }
    }
}
