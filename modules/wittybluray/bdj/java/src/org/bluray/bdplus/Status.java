package org.bluray.bdplus;
/** BD+ is not present on this player, and never will be: linking this class is all that works. */
public class Status {
    public static final int NOT_PRESENT = 0;
    public static int getStatus() { return NOT_PRESENT; }
    public static boolean isPresent() { return false; }
}
