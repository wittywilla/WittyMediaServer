package javax.media;
/** A media format: an encoding name and the Java type its data comes in. */
public class Format implements Cloneable {
    public static final int NOT_SPECIFIED = -1;
    public static final Class<?> byteArray = byte[].class, shortArray = short[].class, intArray = int[].class;
    protected String encoding;
    protected Class<?> dataType = byteArray;
    public Format(String encoding) { this.encoding = encoding; }
    public Format(String encoding, Class<?> dataType) { this.encoding = encoding; this.dataType = dataType; }
    public String getEncoding() { return encoding; }
    public Class<?> getDataType() { return dataType; }
    public boolean isSameEncoding(Format f) { return f != null && isSameEncoding(f.encoding); }
    public boolean isSameEncoding(String e) { return encoding == null ? e == null : encoding.equalsIgnoreCase(e); }
    public boolean matches(Format f) { return f != null && isSameEncoding(f) && (dataType == null || f.dataType == null || dataType == f.dataType); }
    public boolean intersects(Format f) { return matches(f); }
    public Format relax() { return (Format) clone(); }
    public Object clone() {
        try {
            return super.clone();
        } catch (CloneNotSupportedException e) {
            return new Format(encoding, dataType);
        }
    }
    public boolean equals(Object o) { return o instanceof Format && ((Format) o).dataType == dataType && isSameEncoding(((Format) o).encoding); }
    public int hashCode() { return encoding == null ? 0 : encoding.toLowerCase().hashCode(); }
    public String toString() { return String.valueOf(encoding); }
}
