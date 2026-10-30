package auth

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// A browser can send another server's (or an expired) cookie beside the
// token the page put in a media address: every token is offered, most
// explicit first.
func TestTokensOrder(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/items/1/poster?token=fromquery", nil)
	r.AddCookie(&http.Cookie{Name: CookieName, Value: "stalecookie"})
	if got, want := Tokens(r), []string{"fromquery", "stalecookie"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Tokens = %v, want %v", got, want)
	}
	r.Header.Set("Authorization", "Bearer fromheader")
	if got := TokenFrom(r); got != "fromheader" {
		t.Fatalf("TokenFrom = %q, want the header's token", got)
	}
	if got := Tokens(httptest.NewRequest("GET", "/", nil)); len(got) != 0 {
		t.Fatalf("no credentials: Tokens = %v", got)
	}
}
