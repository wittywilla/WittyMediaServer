package witty.bdj;

import java.io.ByteArrayOutputStream;
import java.io.DataOutputStream;
import java.io.IOException;
import java.util.HashMap;
import java.util.Map;

/**
 * Rewrites a disc's class file before it is defined: names in the constant pool
 * that have a checked replacement are renamed, and the few virtual calls whose
 * result would escape the sandbox become static calls into witty.shim.Patch.
 * Stack layout and code length do not change, so the rest of the file stands.
 */
final class ClassPatch {
    private ClassPatch() {}

    private static final String PATCH = "witty/shim/Patch";
    /** owner.name+descriptor (after renaming); owner "*" = any. */
    private static final Map<String, String> CALLS = new HashMap<String, String>();
    static {
        CALLS.put("java/lang/Class.getResource(Ljava/lang/String;)Lwitty/shim/URL;", "getResource(Ljava/lang/Class;Ljava/lang/String;)Lwitty/shim/URL;");
        CALLS.put("java/lang/Class.getClassLoader()Lwitty/shim/Loader;", "getClassLoader(Ljava/lang/Class;)Lwitty/shim/Loader;");
        CALLS.put("*.getContextClassLoader()Lwitty/shim/Loader;", "getContextClassLoader(Ljava/lang/Thread;)Lwitty/shim/Loader;");
        CALLS.put("*.getToolkit()Lwitty/shim/Toolkit;", "getToolkit(Ljava/awt/Component;)Lwitty/shim/Toolkit;");
        // These JDK methods take a type renamed here, so as named by a disc they would not link.
        CALLS.put("*.printStackTrace(Lwitty/shim/PrintStream;)V", "printStackTrace(Ljava/lang/Object;Lwitty/shim/PrintStream;)V");
        CALLS.put("*.printStackTrace(Lwitty/shim/PrintWriter;)V", "printStackTrace(Ljava/lang/Object;Lwitty/shim/PrintWriter;)V");
        for (String owner : new String[] {"java/util/Properties", "java/awt/Component", "java/awt/Container"}) {
            CALLS.put(owner + ".list(Lwitty/shim/PrintStream;)V", "list(Ljava/lang/Object;Lwitty/shim/PrintStream;)V");
            CALLS.put(owner + ".list(Lwitty/shim/PrintWriter;)V", "list(Ljava/lang/Object;Lwitty/shim/PrintWriter;)V");
        }
    }

    static byte[] patch(byte[] c) {
        if (c.length < 10 || u4(c, 0) != 0xCAFEBABE) throw new IllegalArgumentException("not a class file");
        int count = u2(c, 8);
        int[] at = new int[count];
        int[] tag = new int[count];
        String[] utf = new String[count];
        int p = 10;
        for (int i = 1; i < count; i++) {
            int t = c[p] & 255;
            tag[i] = t;
            at[i] = p;
            switch (t) {
                case 1:
                    int l = u2(c, p + 1);
                    utf[i] = utf8(c, p + 3, l);
                    p += 3 + l;
                    break;
                case 3: case 4: case 9: case 10: case 11: case 12: case 17: case 18:
                    p += 5;
                    break;
                case 5: case 6:
                    p += 9;
                    i++;
                    break;
                case 7: case 8: case 16: case 19: case 20:
                    p += 3;
                    break;
                case 15:
                    p += 4;
                    break;
                default:
                    throw new IllegalArgumentException("bad constant pool tag " + t);
            }
        }
        int poolEnd = p;

        // Rename.
        String[] renamed = new String[count];
        boolean[] isClassName = new boolean[count];
        for (int i = 1; i < count; i++) if (tag[i] == 7) isClassName[u2(c, at[i] + 1)] = true;
        for (int i = 1; i < count; i++) {
            if (tag[i] != 1) continue;
            String s = utf[i];
            String r = s;
            if (isClassName[i] && Sandbox.RENAME.containsKey(s)) r = Sandbox.RENAME.get(s);
            else if (s.indexOf(';') > 0 && (s.charAt(0) == '(' || s.charAt(0) == 'L' || s.charAt(0) == '[')) {
                for (Map.Entry<String, String> e : Sandbox.RENAME.entrySet()) {
                    String from = "L" + e.getKey() + ";";
                    if (r.indexOf(from) >= 0) r = r.replace(from, "L" + e.getValue() + ";");
                }
            }
            renamed[i] = r;
        }

        // Find the calls to replace, and add what their replacements need.
        ByteArrayOutputStream extra = new ByteArrayOutputStream();
        int next = count;
        Map<Integer, Integer> replace = new HashMap<Integer, Integer>();
        int patchClass = 0, patchName = 0;
        try {
            DataOutputStream ex = new DataOutputStream(extra);
            for (int i = 1; i < count; i++) {
                if (tag[i] != 10) continue;
                String owner = renamed[u2(c, at[u2(c, at[i] + 1)] + 1)];
                int nt = u2(c, at[i] + 3);
                String name = renamed[u2(c, at[nt] + 1)], desc = renamed[u2(c, at[nt] + 3)];
                String to = CALLS.get(owner + "." + name + desc);
                if (to == null) to = CALLS.get("*." + name + desc);
                if (to == null) continue;
                if (patchClass == 0) {
                    ex.writeByte(1);
                    ex.writeUTF(PATCH);
                    patchName = next++;
                    ex.writeByte(7);
                    ex.writeShort(patchName);
                    patchClass = next++;
                }
                int paren = to.indexOf('(');
                ex.writeByte(1);
                ex.writeUTF(to.substring(0, paren));
                int un = next++;
                ex.writeByte(1);
                ex.writeUTF(to.substring(paren));
                int ud = next++;
                ex.writeByte(12);
                ex.writeShort(un);
                ex.writeShort(ud);
                int newNt = next++;
                ex.writeByte(10);
                ex.writeShort(patchClass);
                ex.writeShort(newNt);
                replace.put(i, next++);
            }
        } catch (IOException e) {
            throw new IllegalStateException(e);
        }

        byte[] rest = new byte[c.length - poolEnd];
        System.arraycopy(c, poolEnd, rest, 0, rest.length);
        if (!replace.isEmpty()) patchCode(rest, utf, replace);

        ByteArrayOutputStream out = new ByteArrayOutputStream(c.length + 256);
        try {
            DataOutputStream o = new DataOutputStream(out);
            o.write(c, 0, 8);
            o.writeShort(next);
            for (int i = 1; i < count; i++) {
                if (tag[i] == 0) continue;
                if (tag[i] == 1) {
                    o.writeByte(1);
                    if (renamed[i].equals(utf[i])) {
                        int l = u2(c, at[i] + 1);
                        o.write(c, at[i] + 1, 2 + l);
                    } else {
                        o.writeUTF(renamed[i]);
                    }
                    continue;
                }
                int end = i + 1 < count ? nextAt(at, tag, i, poolEnd) : poolEnd;
                o.write(c, at[i], end - at[i]);
            }
            extra.writeTo(o);
            o.write(rest);
        } catch (IOException e) {
            throw new IllegalStateException(e);
        }
        return out.toByteArray();
    }

    private static int nextAt(int[] at, int[] tag, int i, int poolEnd) {
        for (int k = i + 1; k < at.length; k++) if (tag[k] != 0) return at[k];
        return poolEnd;
    }

    /** Walks the methods after the constant pool, turning invokevirtual into invokestatic. */
    private static void patchCode(byte[] b, String[] utf, Map<Integer, Integer> replace) {
        int p = 6;
        p += 2 + 2 * u2(b, p); // interfaces
        // Fields.
        int n = u2(b, p);
        p += 2;
        for (int i = 0; i < n; i++) p = skipMember(b, p);
        n = u2(b, p);
        p += 2;
        for (int i = 0; i < n; i++) {
            int attrs = u2(b, p + 6);
            int q = p + 8;
            for (int a = 0; a < attrs; a++) {
                int nameIdx = u2(b, q);
                int len = u4(b, q + 2);
                if (nameIdx < utf.length && "Code".equals(utf[nameIdx])) {
                    int codeLen = u4(b, q + 10);
                    patchInstructions(b, q + 14, codeLen, replace);
                }
                q += 6 + len;
            }
            p = q;
        }
    }

    private static int skipMember(byte[] b, int p) {
        int attrs = u2(b, p + 6);
        int q = p + 8;
        for (int a = 0; a < attrs; a++) q += 6 + u4(b, q + 2);
        return q;
    }

    private static void patchInstructions(byte[] b, int start, int len, Map<Integer, Integer> replace) {
        for (int pc = 0; pc < len; ) {
            int op = b[start + pc] & 255;
            if (op == 0xb6) {
                int idx = u2(b, start + pc + 1);
                Integer to = replace.get(idx);
                if (to != null) {
                    b[start + pc] = (byte) 0xb8;
                    b[start + pc + 1] = (byte) (to >> 8);
                    b[start + pc + 2] = (byte) (int) to;
                }
            }
            pc += length(b, start, pc, op);
        }
    }

    private static int length(byte[] b, int start, int pc, int op) {
        switch (op) {
            case 0xaa: { // tableswitch
                int q = (pc + 4) & ~3;
                int lo = u4(b, start + q + 4), hi = u4(b, start + q + 8);
                return q + 12 + 4 * (hi - lo + 1) - pc;
            }
            case 0xab: { // lookupswitch
                int q = (pc + 4) & ~3;
                int np = u4(b, start + q + 4);
                return q + 8 + 8 * np - pc;
            }
            case 0xc4: // wide
                return (b[start + pc + 1] & 255) == 0x84 ? 6 : 4;
            default:
                return LENGTHS[op];
        }
    }

    private static final int[] LENGTHS = new int[256];
    static {
        java.util.Arrays.fill(LENGTHS, 1);
        int[] two = {0x10, 0x12, 0x15, 0x16, 0x17, 0x18, 0x19, 0x36, 0x37, 0x38, 0x39, 0x3a, 0xa9, 0xbc};
        int[] three = {0x11, 0x13, 0x14, 0x84, 0x99, 0x9a, 0x9b, 0x9c, 0x9d, 0x9e, 0x9f, 0xa0, 0xa1, 0xa2, 0xa3, 0xa4, 0xa5, 0xa6, 0xa7, 0xa8, 0xb2, 0xb3, 0xb4, 0xb5, 0xb6, 0xb7, 0xb8, 0xbb, 0xbd, 0xc0, 0xc1, 0xc6, 0xc7};
        for (int i = 0; i < two.length; i++) LENGTHS[two[i]] = 2;
        for (int i = 0; i < three.length; i++) LENGTHS[three[i]] = 3;
        LENGTHS[0xc5] = 4;
        LENGTHS[0xb9] = 5;
        LENGTHS[0xba] = 5;
        LENGTHS[0xc8] = 5;
        LENGTHS[0xc9] = 5;
    }

    private static int u2(byte[] b, int p) { return (b[p] & 255) << 8 | (b[p + 1] & 255); }
    private static int u4(byte[] b, int p) { return (b[p] & 255) << 24 | (b[p + 1] & 255) << 16 | (b[p + 2] & 255) << 8 | (b[p + 3] & 255); }

    private static String utf8(byte[] b, int p, int l) {
        try {
            return new java.io.DataInputStream(new java.io.ByteArrayInputStream(b, p - 2, l + 2)).readUTF();
        } catch (IOException e) {
            // A name this check cannot read must not reach the JVM, which may read it differently.
            throw new IllegalArgumentException("bad text in a class file");
        }
    }
}
