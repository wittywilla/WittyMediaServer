package org.dvb.event;
public class OverallRepository extends UserEventRepository {
    public OverallRepository() { super("overall"); for (int c = 0; c < 600; c++) addKey(c); }
    public OverallRepository(String n) { super(n); for (int c = 0; c < 600; c++) addKey(c); }
}
