// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"strconv"
	"strings"
	"testing"
)

type withBackdrop struct{ oneItem }

func (withBackdrop) Subject() (*Item, error) {
	return &Item{ID: "7", Kind: "movie", Title: "Seven", Images: map[string]string{"backdrop": "/api/items/7/backdrop", "poster": "/api/items/7/poster"}}, nil
}

// A title's backdrop spans the window, so it is asked for at the window's width at the
// device's scale, bucketed like every other picture: never more than the window needs.
func TestDetailBackdropFollowsTheWindow(t *testing.T) {
	cases := []struct {
		name      string
		ff, class string
		viewport  Viewport
		wantWidth int
	}{
		{name: "small phone", ff: FormPhone, viewport: Viewport{Width: 360, Height: 780}, wantWidth: 1280},
		{name: "phone, no viewport", ff: FormPhone, wantWidth: 1280},
		{name: "large phone", ff: FormPhone, viewport: Viewport{Width: 430, Height: 932}, wantWidth: 1920},
		{name: "tablet", ff: FormTablet, viewport: Viewport{Width: 820, Height: 1180}, wantWidth: 1920},
		{name: "narrow web window", ff: FormWeb, viewport: Viewport{Width: 600, Height: 800}, wantWidth: 1280},
		{name: "wide web window", ff: FormWeb, viewport: Viewport{Width: 1920, Height: 1080}, wantWidth: 3840},
		{name: "HD television", ff: FormTV, wantWidth: 1920},
		{name: "4K television", ff: FormTV, class: "4k", wantWidth: 3840},
	}
	page, _ := StockPage("detail")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &RenderContext{Profile: ProfileFor(tc.ff, tc.class), Viewport: tc.viewport, Resolver: withBackdrop{}, Vars: map[string]string{"item": "7"}}
			doc, err := Render(page, ctx)
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			var walk func(n *Node)
			walk = func(n *Node) {
				if n == nil || got != nil {
					return
				}
				if n.Type == "DetailHeader" {
					if imgs, ok := n.Props["images"].(map[string]any); ok {
						got, _ = imgs["backdrop"].(map[string]any)
					}
				}
				for i := range n.Children {
					walk(&n.Children[i])
				}
			}
			walk(&doc.Root)
			if got == nil {
				t.Fatal("no backdrop")
			}
			if w, _ := got["width"].(int); w != tc.wantWidth || !strings.HasSuffix(got["url"].(string), "w="+strconv.Itoa(tc.wantWidth)) {
				t.Errorf("backdrop %v, want width %d", got, tc.wantWidth)
			}
		})
	}
}
