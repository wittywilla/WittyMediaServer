package javax.tv.service.navigation;
/** Preference lists (favourites) do not exist on a disc player: nothing passes. */
public class PreferenceFilter extends ServiceFilter {
    private final String name;
    public PreferenceFilter(String preferenceName) { name = preferenceName; }
    public String getFilterValue() { return name; }
    public boolean accept(javax.tv.service.Service s) { return false; }
}
