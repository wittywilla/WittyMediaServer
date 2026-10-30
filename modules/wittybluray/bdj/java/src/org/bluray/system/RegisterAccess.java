package org.bluray.system;

import witty.bdj.Host;

public final class RegisterAccess {
    private static final RegisterAccess it = new RegisterAccess();
    public static final int PSR_TITLE_NR = 4, PSR_CHAPTER_NR = 5, PSR_PLAYLIST_ID = 6, PSR_PLAYITEM_ID = 7, PSR_PLAYBACK_TIME = 8, PSR_NAV_TIMER = 9, PSR_SELECTED_BUTTON = 10, PSR_MENU_PAGE = 11, PSR_STYLE = 12, PSR_PARENTAL = 13;
    private RegisterAccess() {}
    public static RegisterAccess getInstance() { return it; }
    public int getPSR(int n) { if (n < 0 || n > 127) throw new IllegalArgumentException(); return Host.PSR[n]; }
    public int getGPR(int n) { if (n < 0 || n > 4095) throw new IllegalArgumentException(); return Host.GPR[n]; }
    public void setGPR(int n, int v) { if (n < 0 || n > 4095) throw new IllegalArgumentException(); Host.setGPR(n, v); }
    public void setPSR(int n, int v) {
        // Same rule as a disc's commands: the player's own settings are not the disc's to change.
        if (n < 0 || n > 127 || n == 13 || (n >= 15 && n <= 21) || (n >= 23 && n <= 31) || (n >= 48 && n <= 61)) throw new IllegalArgumentException("PSR" + n);
        Host.setPSR(n, v);
    }
}
