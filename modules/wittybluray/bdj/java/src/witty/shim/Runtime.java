package witty.shim;

import java.io.IOException;

public final class Runtime {
    private static final Runtime it = new Runtime();
    private Runtime() {}
    public static Runtime getRuntime() { return it; }
    public long totalMemory() { return java.lang.Runtime.getRuntime().totalMemory(); }
    public long freeMemory() { return java.lang.Runtime.getRuntime().freeMemory(); }
    public long maxMemory() { return java.lang.Runtime.getRuntime().maxMemory(); }
    public int availableProcessors() { return 1; }
    public void gc() { java.lang.System.gc(); }
    public void runFinalization() {}
    public void exit(int c) {}
    public void halt(int c) {}
    public void addShutdownHook(Thread t) {}
    public void traceInstructions(boolean b) {}
    public void traceMethodCalls(boolean b) {}
    public void load(String f) { throw new UnsatisfiedLinkError("native code is not available to disc applications"); }
    public void loadLibrary(String f) { throw new UnsatisfiedLinkError("native code is not available to disc applications"); }
}
