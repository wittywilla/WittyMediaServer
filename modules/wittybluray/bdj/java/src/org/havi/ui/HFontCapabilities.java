package org.havi.ui;
/** What a font can draw, asked of the font itself. */
public class HFontCapabilities {
    public static final int BASIC_LATIN = 1, LATIN_1_SUPPLEMENT = 2, LATIN_EXTENDED_A = 3, LATIN_EXTENDED_B = 4, GREEK_AND_COPTIC = 5, CYRILLIC = 6, ARMENIAN = 7, HEBREW = 8, ARABIC = 9, THAI = 10,
        HIRAGANA = 11, KATAKANA = 12, CJK_UNIFIED_IDEOGRAPHS = 13, HANGUL_SYLLABLES = 14;
    private static final int[][] PROBES = {{BASIC_LATIN, 'A'}, {LATIN_1_SUPPLEMENT, 0xE9}, {LATIN_EXTENDED_A, 0x100}, {LATIN_EXTENDED_B, 0x180}, {GREEK_AND_COPTIC, 0x3B1}, {CYRILLIC, 0x416}, {ARMENIAN, 0x531}, {HEBREW, 0x5D0}, {ARABIC, 0x627}, {THAI, 0xE01}, {HIRAGANA, 0x3042}, {KATAKANA, 0x30A2}, {CJK_UNIFIED_IDEOGRAPHS, 0x4E00}, {HANGUL_SYLLABLES, 0xAC00}};
    protected HFontCapabilities() {}
    public static int[] getSupportedCharacterRanges(java.awt.Font f) {
        if (f == null) return null;
        int[] o = new int[PROBES.length];
        int n = 0;
        for (int i = 0; i < PROBES.length; i++) if (f.canDisplay((char) PROBES[i][1])) o[n++] = PROBES[i][0];
        return n == 0 ? null : java.util.Arrays.copyOf(o, n);
    }
    public static boolean isCharAvailable(java.awt.Font f, char c) { return f != null && f.canDisplay(c); }
}
