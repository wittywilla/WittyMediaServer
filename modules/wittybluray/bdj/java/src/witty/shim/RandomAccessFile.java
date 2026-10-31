package witty.shim;

import java.io.Closeable;
import java.io.DataInput;
import java.io.DataOutput;
import java.io.IOException;

/**
 * java.io.RandomAccessFile for disc code. It wraps the real one rather than
 * extending it: the real one's final methods write without passing through
 * anything a subclass could count, and growing a file is what the quota charges,
 * however it is done (a seek past the end, setLength, or plain writes).
 */
public class RandomAccessFile implements DataInput, DataOutput, Closeable {
    private final java.io.RandomAccessFile f;
    public RandomAccessFile(String path, String mode) throws java.io.FileNotFoundException { f = new java.io.RandomAccessFile(check(path, mode), mode); }
    public RandomAccessFile(File file, String mode) throws java.io.FileNotFoundException { this(file.getPath(), mode); }
    private static java.io.File check(String p, String mode) throws java.io.FileNotFoundException {
        try {
            return Roots.resolve(p, mode.indexOf('w') >= 0);
        } catch (java.io.FileNotFoundException e) {
            throw e;
        } catch (IOException e) {
            throw new java.io.FileNotFoundException(p);
        }
    }
    private void grow(long n) throws IOException {
        long more = f.getFilePointer() + n - f.length();
        if (more > 0) Roots.charge(more);
    }
    public int read() throws IOException { return f.read(); }
    public int read(byte[] b) throws IOException { return f.read(b); }
    public int read(byte[] b, int o, int l) throws IOException { return f.read(b, o, l); }
    public long getFilePointer() throws IOException { return f.getFilePointer(); }
    public void seek(long p) throws IOException { f.seek(p); }
    public long length() throws IOException { return f.length(); }
    public void setLength(long n) throws IOException {
        long old = f.length();
        if (n > old) Roots.charge(n - old);
        f.setLength(n);
        if (n < old) Roots.refund(old - n);
    }
    public void close() throws IOException { f.close(); }
    public void readFully(byte[] b) throws IOException { f.readFully(b); }
    public void readFully(byte[] b, int o, int l) throws IOException { f.readFully(b, o, l); }
    public int skipBytes(int n) throws IOException { return f.skipBytes(n); }
    public boolean readBoolean() throws IOException { return f.readBoolean(); }
    public byte readByte() throws IOException { return f.readByte(); }
    public int readUnsignedByte() throws IOException { return f.readUnsignedByte(); }
    public short readShort() throws IOException { return f.readShort(); }
    public int readUnsignedShort() throws IOException { return f.readUnsignedShort(); }
    public char readChar() throws IOException { return f.readChar(); }
    public int readInt() throws IOException { return f.readInt(); }
    public long readLong() throws IOException { return f.readLong(); }
    public float readFloat() throws IOException { return f.readFloat(); }
    public double readDouble() throws IOException { return f.readDouble(); }
    public String readLine() throws IOException { return f.readLine(); }
    public String readUTF() throws IOException { return f.readUTF(); }
    public void write(int b) throws IOException { grow(1); f.write(b); }
    public void write(byte[] b) throws IOException { grow(b.length); f.write(b); }
    public void write(byte[] b, int o, int l) throws IOException { grow(l); f.write(b, o, l); }
    public void writeBoolean(boolean v) throws IOException { grow(1); f.writeBoolean(v); }
    public void writeByte(int v) throws IOException { grow(1); f.writeByte(v); }
    public void writeShort(int v) throws IOException { grow(2); f.writeShort(v); }
    public void writeChar(int v) throws IOException { grow(2); f.writeChar(v); }
    public void writeInt(int v) throws IOException { grow(4); f.writeInt(v); }
    public void writeLong(long v) throws IOException { grow(8); f.writeLong(v); }
    public void writeFloat(float v) throws IOException { grow(4); f.writeFloat(v); }
    public void writeDouble(double v) throws IOException { grow(8); f.writeDouble(v); }
    public void writeBytes(String s) throws IOException { grow(s.length()); f.writeBytes(s); }
    public void writeChars(String s) throws IOException { grow(2L * s.length()); f.writeChars(s); }
    public void writeUTF(String s) throws IOException { grow(2L + 3L * s.length()); f.writeUTF(s); }
}
