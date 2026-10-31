package javax.tv.service.navigation;
public interface ServiceIterator {
    javax.tv.service.Service nextService();
    javax.tv.service.Service previousService();
    boolean hasNext();
    boolean hasPrevious();
    void toBeginning();
    void toEnd();
}
