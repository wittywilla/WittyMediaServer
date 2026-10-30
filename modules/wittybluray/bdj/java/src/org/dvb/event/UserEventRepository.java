package org.dvb.event;
public class UserEventRepository extends RepositoryDescriptor {
    private final java.util.List<UserEvent> list = new java.util.ArrayList<UserEvent>();
    private final String name;
    public UserEventRepository(String n) { name = n; }
    public void addUserEvent(UserEvent e) { list.add(e); }
    public UserEvent[] getUserEvent() { return list.toArray(new UserEvent[list.size()]); }
    public void removeUserEvent(UserEvent e) { list.remove(e); }
    public void addKey(int code) { list.add(new UserEvent(this, UserEvent.UEF_KEY_EVENT, 0, code, 0, 0)); }
    public void removeKey(int code) { for (int i = list.size() - 1; i >= 0; i--) if (list.get(i).getCode() == code) list.remove(i); }
    public void addAllNumericKeys() { for (int c = 48; c <= 57; c++) addKey(c); }
    public void addAllColourKeys() { for (int c = 403; c <= 406; c++) addKey(c); }
    public void addAllArrowKeys() { for (int c = 37; c <= 40; c++) addKey(c); }
    public void removeAllNumericKeys() { for (int c = 48; c <= 57; c++) removeKey(c); }
    public void removeAllColourKeys() { for (int c = 403; c <= 406; c++) removeKey(c); }
    public void removeAllArrowKeys() { for (int c = 37; c <= 40; c++) removeKey(c); }
    public String getName() { return name; }
    boolean has(int code) { for (UserEvent e : list) if (e.getCode() == code) return true; return false; }
}
