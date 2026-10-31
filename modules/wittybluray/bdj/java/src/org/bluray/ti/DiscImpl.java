package org.bluray.ti;

/** The disc this player was started on. */
final class DiscImpl implements Disc {
    private String id;
    private Title[] titles;
    public synchronized String getId() {
        if (id != null) return id;
        // The certificate file: the disc's number follows the organisation's at byte 44.
        byte[] b = DiscFiles.head("CERTIFICATE/id.bdmv", 60);
        StringBuilder s = new StringBuilder();
        if (b != null && b.length == 60) for (int i = 44; i < 60; i++) { s.append(Character.forDigit((b[i] >> 4) & 15, 16)); s.append(Character.forDigit(b[i] & 15, 16)); }
        id = s.toString();
        return id;
    }
    public String getName() { return getId(); }
    public synchronized Title[] getTitles() {
        if (titles == null) {
            // The index: past its header, first play and top menu entries of 12 bytes each, then the count of titles.
            int n = 0;
            byte[] h = DiscFiles.head("BDMV/index.bdmv", 4096);
            if (h != null && h.length >= 12) {
                int at = DiscFiles.u32(h, 8) + 4 + 24;
                if (at >= 0 && at + 2 <= h.length) n = Math.min(999, DiscFiles.u16(h, at));
            }
            titles = new Title[n];
            for (int i = 0; i < n; i++) titles[i] = new TitleImpl(i + 1);
        }
        return (Title[]) titles.clone();
    }
}
