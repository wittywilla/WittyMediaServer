package server

import (
	"context"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/brandname"
)

type brandStatus struct {
	BrandName      string           `json:"brandName"`
	BrandColouring string           `json:"brandColouring"`
	BrandParts     []brandname.Part `json:"brandParts"`
}

func marked(parts []brandname.Part) string {
	s := ""
	for _, p := range parts {
		if p.Accent {
			s += "[" + p.Text + "]"
		} else {
			s += p.Text
		}
	}
	return s
}

// The name's colouring is kept with it on the server, so every device and app
// draws it the same, and an app that only knows the name gets it ready to draw.
func TestBrandColouring(t *testing.T) {
	ts := newContractServer(t)
	_, admin := ts.account(t, "Boss", "adult", true)
	status := func() brandStatus { return decode[brandStatus](t, ts.do(t, "GET", "/api/status", "", nil)) }

	if s := status(); s.BrandName != "" || s.BrandColouring != "auto" || marked(s.BrandParts) != "Witty[Media]Server" {
		t.Errorf("unnamed: %+v", s)
	}
	// A server renamed before colouring existed gets the automatic split.
	ts.do(t, "PUT", "/api/settings", admin, map[string]string{"brandName": "Willow's Movies"})
	if s := status(); s.BrandColouring != "auto" || marked(s.BrandParts) != "Willow's [Movies]" {
		t.Errorf("renamed: %+v", s)
	}
	ts.do(t, "PUT", "/api/settings", admin, map[string]string{"brandName": "MyHomeTheater", "brandColouring": "pick:0-2"})
	if s := status(); s.BrandColouring != "pick:0-2" || marked(s.BrandParts) != "[My]HomeTheater" {
		t.Errorf("picked: %+v", s)
	}
	// Saving the same name again keeps the chosen words.
	ts.do(t, "PUT", "/api/settings", admin, map[string]string{"brandName": "MyHomeTheater"})
	if got := ts.DB.GetSetting(context.Background(), "brandColouring", ""); got != "pick:0-2" {
		t.Errorf("same name: %q", got)
	}
	// A new name from an app that doesn't send the colouring: the chosen words don't fit it.
	ts.do(t, "PUT", "/api/settings", admin, map[string]string{"brandName": "Kim's TV Room"})
	if s := status(); s.BrandColouring != "auto" || marked(s.BrandParts) != "Kim's [TV] Room" {
		t.Errorf("new name: %+v", s)
	}
	ts.do(t, "PUT", "/api/settings", admin, map[string]string{"brandColouring": "one"})
	if s := status(); marked(s.BrandParts) != "Kim's TV Room" {
		t.Errorf("one colour: %+v", s)
	}
	for _, bad := range []string{"pick:9-3", "pick:0-81", "rainbow"} {
		if w := ts.do(t, "PUT", "/api/settings", admin, map[string]string{"brandColouring": bad}); w.Code != 400 {
			t.Errorf("%q: %d", bad, w.Code)
		}
	}
	if got := ts.DB.GetSetting(context.Background(), "brandColouring", ""); got != "one" {
		t.Errorf("after refusals: %q", got)
	}
	// A refused colouring leaves the name as it was too.
	if w := ts.do(t, "PUT", "/api/settings", admin, map[string]string{"brandName": "Elsewhere", "brandColouring": "pick:9-3"}); w.Code != 400 {
		t.Errorf("name with a bad colouring: %d", w.Code)
	}
	if got := ts.DB.GetSetting(context.Background(), "brandName", ""); got != "Kim's TV Room" {
		t.Errorf("name changed by a refused request: %q", got)
	}
	// Only an administrator names the server.
	_, kid := ts.account(t, "Kid", "kids", false)
	if w := ts.do(t, "PUT", "/api/settings", kid, map[string]string{"brandColouring": "auto"}); w.Code != 403 {
		t.Errorf("not an admin: %d", w.Code)
	}
}
