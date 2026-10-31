package javax.media.protocol;
public interface Positionable {
    int RoundUp = 1, RoundDown = 2, RoundNearest = 3;
    javax.media.Time setPosition(javax.media.Time where, int rounding);
    boolean isRandomAccess();
}
