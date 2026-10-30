package org.havi.ui.event;
public class HRcEvent extends java.awt.event.KeyEvent {
    public static final int RC_FIRST = 400, RC_LAST = 480;
    public static final int VK_COLORED_KEY_0 = 403, VK_COLORED_KEY_1 = 404, VK_COLORED_KEY_2 = 405, VK_COLORED_KEY_3 = 406,
        VK_POWER = 409, VK_DIMMER = 410, VK_REWIND = 412, VK_STOP = 413, VK_EJECT_TOGGLE = 414, VK_PLAY = 415, VK_RECORD = 416, VK_FAST_FWD = 417, VK_PLAY_SPEED_UP = 418, VK_PLAY_SPEED_DOWN = 419, VK_PLAY_SPEED_RESET = 420,
        VK_RECORD_SPEED_NEXT = 421, VK_GO_TO_START = 422, VK_GO_TO_END = 423, VK_TRACK_PREV = 424, VK_TRACK_NEXT = 425, VK_RANDOM_TOGGLE = 426, VK_CHANNEL_UP = 427, VK_CHANNEL_DOWN = 428, VK_STORE_FAVORITE_0 = 429,
        VK_RECALL_FAVORITE_0 = 433, VK_CLEAR_FAVORITE_0 = 437, VK_MUTE = 449, VK_VOLUME_UP = 447, VK_VOLUME_DOWN = 448, VK_SUBTITLE = 460, VK_AUDIO = 461, VK_ANGLE = 462;
    public HRcEvent(java.awt.Component s, int id, long when, int mod, int code) { super(s, id, when, mod, code, CHAR_UNDEFINED); }
    public HRcEvent(java.awt.Component s, int id, long when, int mod, int code, char c) { super(s, id, when, mod, code, c); }
}
