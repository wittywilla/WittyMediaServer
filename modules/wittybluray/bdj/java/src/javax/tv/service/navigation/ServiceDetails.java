package javax.tv.service.navigation;
public interface ServiceDetails extends javax.tv.service.SIElement {
    javax.tv.service.Service getService();
    String getLongName();
    javax.tv.service.ServiceType getServiceType();
    DeliverySystemType getDeliverySystemType();
    CAIdentification getCAIdentification();
    javax.tv.service.guide.ProgramSchedule getProgramSchedule();
    javax.tv.service.SIRequest retrieveComponents(javax.tv.service.SIRequestor r);
    javax.tv.service.SIRequest retrieveServiceDescription(javax.tv.service.SIRequestor r);
}
