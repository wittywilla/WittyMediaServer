package witty.shim;

import java.io.OutputStream;

/** Without the constructors that open files. */
public class PrintStream extends java.io.PrintStream {
    public PrintStream(OutputStream o) { super(o); }
    public PrintStream(OutputStream o, boolean flush) { super(o, flush); }
    public PrintStream(OutputStream o, boolean flush, String enc) throws java.io.UnsupportedEncodingException { super(o, flush, enc); }
}
