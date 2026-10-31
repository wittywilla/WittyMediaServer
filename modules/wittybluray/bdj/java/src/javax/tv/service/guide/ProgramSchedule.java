package javax.tv.service.guide;
public interface ProgramSchedule {
    javax.tv.service.SIRequest retrieveCurrentProgramEvent(javax.tv.service.SIRequestor r);
    javax.tv.service.SIRequest retrieveFutureProgramEvents(java.util.Date begin, java.util.Date end, javax.tv.service.SIRequestor r);
    javax.tv.service.SIRequest retrieveFutureProgramEvents(java.util.Date begin, javax.tv.service.SIRequestor r);
    void addListener(ProgramScheduleListener l);
    void removeListener(ProgramScheduleListener l);
}
