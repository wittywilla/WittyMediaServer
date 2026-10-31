package javax.tv.service.navigation;
public interface ServiceComponent extends javax.tv.service.SIElement {
    String getName();
    String getAssociatedLanguage();
    StreamType getStreamType();
    ServiceDetails getServiceDetails();
}
