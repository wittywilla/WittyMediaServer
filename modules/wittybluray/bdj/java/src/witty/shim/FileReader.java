package witty.shim;

public class FileReader extends java.io.InputStreamReader {
    public FileReader(String path) throws java.io.FileNotFoundException { super(new FileInputStream(path)); }
    public FileReader(File f) throws java.io.FileNotFoundException { super(new FileInputStream(f)); }
}
