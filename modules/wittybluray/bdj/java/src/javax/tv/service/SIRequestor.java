package javax.tv.service;
public interface SIRequestor {
    void notifySuccess(SIRetrievable[] result);
    void notifyFailure(SIRequestFailureType reason);
}
