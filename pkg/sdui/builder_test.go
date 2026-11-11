package sdui

import (
	"testing"
)

func TestBuilder_Document(t *testing.T) {
	doc, err := NewDocument("home").
		Theme("midnight").
		ModuleOverride("wittyplay", "accent", "#3ad29f").
		Action("go", NavigateTo("/library")).
		Action("play", PlayItem("item-1")).
		Action("open", OpenModalAction("m")).
		Action("close", CloseModalAction()).
		Action("fetch", FetchFrom("/ui/home")).
		Action("back", BackAction()).
		Meta(Meta{Title: "Home", TTLSeconds: 30}).
		Root(ScreenNode("root").
			Prop("title", "Home").
			Props(map[string]any{"scroll": true}).
			Child(
				StackOf("stack").Child(
					TextOf("greeting", "Hi").Bind("text", "$.user.name"),
					ButtonOf("btn", "Go", "go").If(IsRole("admin")),
					ImageOf("img", "/a.png"),
					nil,
				),
				Row("row").Child(BadgeOf("b", "NEW"), Progress("p", 0.5), SpacerOf("sp", 8), DividerOf("d")),
				GridOf("grid", 4).Module("wittyplay"),
				ShelfOf("shelf", "Shelf"), PosterRowOf("pr", "Row"), Hero("hero"),
				TabsOf("tabs").Child(TabOf("tab", "One")),
				ListOf("list").Child(ListItemOf("li", "Item")),
				VideoOf("vid", "/v.m3u8"), SearchBarOf("sb", "Search"),
				ToolbarOf("tb"), ModalOf("modal", "M"), CarouselOf("car"),
				CardOf("card", Item{ID: "x", Title: "T", Subtitle: "S", ImageURL: "/i.png", Kind: "movie",
					Progress: 0.2, Badges: []string{"4K"}, Module: "wittydvd"}),
			)).
		Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if doc.Version != SchemaVersion || doc.Screen != "home" {
		t.Errorf("header wrong: %+v", doc)
	}
	if doc.Theme == nil || doc.Theme.ID != "midnight" || doc.Theme.ModuleOverrides["wittyplay"]["accent"] != "#3ad29f" {
		t.Errorf("theme wrong: %+v", doc.Theme)
	}
	if len(doc.Actions) != 6 {
		t.Errorf("expected 6 actions, got %d", len(doc.Actions))
	}
	if doc.Meta.Title != "Home" {
		t.Errorf("meta not set")
	}
	if doc.Root.Props["scroll"] != true || doc.Root.Props["title"] != "Home" {
		t.Errorf("root props wrong: %v", doc.Root.Props)
	}
	stack := doc.Root.Children[0]
	if stack.Style == nil || stack.Style.Direction != "column" || len(stack.Children) != 3 {
		t.Errorf("stack wrong: %+v", stack)
	}
	if stack.Children[0].Bindings["text"].Path != "$.user.name" {
		t.Errorf("binding missing")
	}
	if stack.Children[1].Visibility == nil || stack.Children[1].Visibility.Role != "admin" || stack.Children[1].Action != "go" {
		t.Errorf("button wrong: %+v", stack.Children[1])
	}
	row := doc.Root.Children[1]
	if row.Style.Direction != "row" {
		t.Errorf("row direction wrong")
	}
	grid := doc.Root.Children[2]
	if grid.Style.Columns != 4 || grid.Style.Module != "wittyplay" {
		t.Errorf("grid wrong: %+v", grid.Style)
	}
	card := doc.Root.Children[len(doc.Root.Children)-1]
	if card.Props["itemId"] != "x" || card.Props["module"] != "wittydvd" || card.Style.Module != "wittydvd" {
		t.Errorf("card wrong: %+v", card)
	}
	if b, ok := card.Props["badges"].([]string); !ok || b[0] != "4K" {
		t.Errorf("badges wrong: %v", card.Props["badges"])
	}
}

func TestBuilder_ModuleOverrideWithoutTheme(t *testing.T) {
	d := NewDocument("x").ModuleOverride("wittytv", "accent", "#fff").Root(ScreenNode("r")).Document()
	if d.Theme == nil || d.Theme.ID != "default" {
		t.Fatalf("expected default theme, got %+v", d.Theme)
	}
}

func TestBuilder_BuildFails(t *testing.T) {
	_, err := NewDocument("").Root(StackOf("r")).Build()
	if err == nil {
		t.Fatal("expected validation failure")
	}
	_, err = NewDocument("x").Build() // no root
	if err == nil {
		t.Fatal("expected failure for missing root")
	}
}

func TestBuilder_NodeIsCopy(t *testing.T) {
	nb := TextOf("t", "a")
	n1 := nb.Node()
	nb.Prop("text", "b")
	n2 := nb.Node()
	if n1.Props["text"] != "a" || n2.Props["text"] != "b" {
		t.Error("Node() should return independent copies")
	}
	n1.Props["text"] = "z"
	if nb.Node().Props["text"] != "b" {
		t.Error("mutating returned node leaked into the builder")
	}
	cn := StackOf("s").ChildNodes(n1, n2).Node()
	if len(cn.Children) != 2 {
		t.Error("ChildNodes did not append")
	}
	if BindWithDefault := nb.BindWith("alt", Binding{Path: "$.a", Default: "d"}).Node(); BindWithDefault.Bindings["alt"].Default != "d" {
		t.Error("BindWith lost default")
	}
}

func TestConditionHelpers(t *testing.T) {
	c := All(HasFeature("f"), IsRole("admin"), OnPlatform("tv"), ModuleEnabled("wittyplay"), HasFlag("x"),
		InLocale("en"), MinWidth(100), MaxWidth(200), Any(IsRole("a"), IsRole("b")), Not(IsRole("guest")))
	if len(c.All) != 10 {
		t.Fatalf("expected 10 children, got %d", len(c.All))
	}
	if c.All[0].Feature != "f" || c.All[1].Role != "admin" || c.All[2].Platform != "tv" || c.All[3].Module != "wittyplay" ||
		c.All[4].Flag != "x" || c.All[5].Locale != "en" || c.All[6].MinWidth != 100 || c.All[7].MaxWidth != 200 ||
		len(c.All[8].Any) != 2 || c.All[9].Not == nil || c.All[9].Not.Role != "guest" {
		t.Errorf("helpers produced wrong condition: %+v", c)
	}
}
