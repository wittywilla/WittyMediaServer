package org.dvb.test;

/** Test reports go to the player log. */
public class DVBTest {
    public static final int OK = 0, FAIL = 1, UNRESOLVED = 2;
    private DVBTest() {}
    public static void log(String message) { witty.shim.System.out.println("DVBTest: " + message); }
    public static void log(String message, int code) { witty.shim.System.out.println("DVBTest: " + message + " (" + code + ")"); }
    public static void ping(String message) { log(message); }
    public static void terminate(int code) { witty.shim.System.out.println("DVBTest: terminate " + code); }
}
