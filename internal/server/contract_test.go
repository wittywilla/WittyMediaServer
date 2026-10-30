package server

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/dvdsession"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/playback"
	"github.com/wittywilla/wittymediaserver/internal/themes"
	"github.com/wittywilla/wittymediaserver/internal/update"
)

// The client contract stays in step with the code: docs/openapi.json and
// docs/CLIENT-API.md must describe every route, and what they promise about
// sign-in is tried against the real server. When one of these fails after you
// added a route, the message says what to add.

const (
	openapiPath = "../../docs/openapi.json"
	clientDoc   = "../../docs/CLIENT-API.md"
)

// apiRoute is one route as the code registers it.
type apiRoute struct {
	method, path string // "/items/{id}", without /api
	raw          string // the pattern as written
	public       bool
	auth         string // public | user | admin
}

func (r apiRoute) pattern() string { return r.method + " /api" + r.path }

// wildcard {file...} is written {file} in OpenAPI.
func docPath(p string) string { return strings.ReplaceAll(p, "...}", "}") }

var routePattern = regexp.MustCompile(`^(GET|POST|PUT|DELETE|PATCH|HEAD) /api(/.*)$`)

// codeRoutes reads the routes out of this package's source, wherever they are
// registered and whatever the variable holding the mux is called. Sign-in comes
// from what wraps the handler: a(...)/s.requireUser(...) or s.requireAdmin(...).
func codeRoutes(t *testing.T) []apiRoute {
	t.Helper()
	fset := token.NewFileSet()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var out []apiRoute
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (sel.Sel.Name != "Handle" && sel.Sel.Name != "HandleFunc") {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			pat, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			m := routePattern.FindStringSubmatch(pat)
			if m == nil {
				return true
			}
			r := apiRoute{method: m[1], path: m[2], raw: pat, auth: "public"}
			if wrap, ok := call.Args[1].(*ast.CallExpr); ok {
				switch fn := wrap.Fun.(type) {
				case *ast.Ident:
					if fn.Name == "a" {
						r.auth = "user"
					}
				case *ast.SelectorExpr:
					switch fn.Sel.Name {
					case "requireUser":
						r.auth = "user"
					case "requireAdmin":
						r.auth = "admin"
					}
				}
			}
			r.public = r.auth == "public"
			out = append(out, r)
			return true
		})
	}
	if len(out) < 100 {
		t.Fatalf("found only %d routes: has registration changed?", len(out))
	}
	return out
}

// documentedRoute is one operation from openapi.json.
type documentedRoute struct {
	method, path string
	public       bool
	auth         string
	streamToken  bool
}

func (r documentedRoute) pattern() string { return r.method + " /api" + r.path }

type openAPIDoc struct {
	OpenAPI string                                `json:"openapi"`
	Info    map[string]any                        `json:"info"`
	Servers []map[string]any                      `json:"servers"`
	Paths   map[string]map[string]json.RawMessage `json:"paths"`
	Comps   struct {
		Schemas map[string]json.RawMessage `json:"schemas"`
	} `json:"components"`
}

func readOpenAPI(t *testing.T) *openAPIDoc {
	t.Helper()
	b, err := os.ReadFile(openapiPath)
	if err != nil {
		t.Fatal(err)
	}
	var d openAPIDoc
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatalf("docs/openapi.json is not valid JSON: %v", err)
	}
	return &d
}

type operation struct {
	Summary    string `json:"summary"`
	OperationI string `json:"operationId"`
	Auth       string `json:"x-wms-auth"`
	Stream     bool   `json:"x-wms-stream-token"`
	Security   []any  `json:"security"`
	Parameters []struct {
		Name string `json:"name"`
		In   string `json:"in"`
	} `json:"parameters"`
	Responses map[string]json.RawMessage `json:"responses"`
}

func documentedRoutes(t *testing.T) []documentedRoute {
	t.Helper()
	var out []documentedRoute
	for p, ops := range readOpenAPI(t).Paths {
		for method, raw := range ops {
			var op operation
			if err := json.Unmarshal(raw, &op); err != nil {
				t.Fatalf("%s %s: %v", method, p, err)
			}
			out = append(out, documentedRoute{method: strings.ToUpper(method), path: p, auth: op.Auth, public: op.Auth == "public", streamToken: op.Stream})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].pattern() < out[j].pattern() })
	return out
}

// fillPath puts plausible values into path parameters.
func fillPath(p string, id int64) string {
	return regexp.MustCompile(`\{[^}]+\}`).ReplaceAllStringFunc("/api"+p, func(v string) string {
		switch v {
		case "{file}":
			return "0.vtt"
		case "{name}":
			return "x.png"
		case "{sid}":
			return "nosuchsession"
		case "{kind}":
			return "poster"
		}
		return strconv.FormatInt(id, 10)
	})
}

// A stub ready to paste into docs/openapi.json, to get a new route started.
func openAPIStub(r apiRoute) string {
	var params []string
	for _, m := range regexp.MustCompile(`\{([^.}]+)(?:\.\.\.)?\}`).FindAllStringSubmatch(r.path, -1) {
		params = append(params, fmt.Sprintf(`{"name": %q, "in": "path", "required": true, "schema": {"type": "string"}}`, m[1]))
	}
	sec := `[{"bearer": []}]`
	if r.public {
		sec = `[]`
	}
	return fmt.Sprintf(`"%s": {"%s": {"summary": "...", "tags": ["..."], "x-wms-auth": %q, "security": %s, "parameters": [%s], "responses": {"200": {"description": "...", "content": {"application/json": {"schema": {"type": "object"}}}}, "default": {"$ref": "#/components/responses/Error"}}}}`,
		docPath(r.path), strings.ToLower(r.method), r.auth, sec, strings.Join(params, ", "))
}

// Every route has to be in the OpenAPI file and in CLIENT-API.md, and nothing
// may be documented that doesn't exist.
func TestEveryRouteIsDocumented(t *testing.T) {
	doc, err := os.ReadFile(clientDoc)
	if err != nil {
		t.Fatal(err)
	}
	md := string(doc)
	documented := map[string]documentedRoute{}
	for _, r := range documentedRoutes(t) {
		documented[r.method+" "+r.path] = r
	}
	seen := map[string]bool{}
	for _, r := range codeRoutes(t) {
		key := r.method + " " + docPath(r.path)
		seen[key] = true
		if _, ok := documented[key]; !ok {
			t.Errorf("route %s is missing from docs/openapi.json. Add under \"paths\":\n  %s", r.raw, openAPIStub(r))
		}
		if !strings.Contains(md, "`"+key+"`") {
			t.Errorf("route %s is missing from docs/CLIENT-API.md. Add a table row starting with `%s` to its section", r.raw, key)
		}
	}
	for key := range documented {
		if !seen[key] {
			t.Errorf("docs/openapi.json documents %s, which isn't a route (any more)", key)
		}
	}
}

// What the docs say about sign-in must be what the code does.
func TestDocumentedSignInMatchesTheCode(t *testing.T) {
	documented := map[string]documentedRoute{}
	for _, r := range documentedRoutes(t) {
		documented[r.method+" "+r.path] = r
	}
	for _, r := range codeRoutes(t) {
		d, ok := documented[r.method+" "+docPath(r.path)]
		if !ok {
			continue // reported by TestEveryRouteIsDocumented
		}
		if d.auth != r.auth {
			t.Errorf("%s: docs say sign-in is %q, the code says %q", r.raw, d.auth, r.auth)
		}
	}
}

// ...and the running server must refuse what the docs say it refuses.
func TestSignInIsEnforced(t *testing.T) {
	ts := newContractServer(t)
	_, adult := ts.account(t, "Alex", "adult", false)
	id := ts.item(t, "video", tempMedia(t, "a.mp4", "AAAA"))
	for _, r := range documentedRoutes(t) {
		path := fillPath(r.path, id)
		switch r.auth {
		case "user", "admin":
			if w := ts.do(t, r.method, path, "", nil); w.Code != 401 {
				t.Errorf("%s %s without signing in: %d, want 401", r.method, path, w.Code)
			}
			if w := ts.do(t, r.method, path, "st_nonsense", nil); w.Code != 401 {
				t.Errorf("%s %s with a made up token: %d, want 401", r.method, path, w.Code)
			}
		}
		if r.auth == "admin" {
			if w := ts.do(t, r.method, path, adult, nil); w.Code != 403 {
				t.Errorf("%s %s as a non-admin: %d, want 403", r.method, path, w.Code)
			}
		}
	}
}

// The docs flag exactly the routes a stream token opens.
func TestStreamTokenRoutesAreFlagged(t *testing.T) {
	var s Server
	n := 0
	for _, r := range documentedRoutes(t) {
		code := s.streamRoutes(r.pattern()) != nil
		if code {
			n++
		}
		if code != r.streamToken {
			t.Errorf("%s: x-wms-stream-token is %v, but the code says %v", r.pattern(), r.streamToken, code)
		}
	}
	if n == 0 {
		t.Error("no stream token routes found")
	}
}

// A basic check that the file is well-formed OpenAPI, for the tools reading it.
func TestOpenAPIIsWellFormed(t *testing.T) {
	d := readOpenAPI(t)
	if !strings.HasPrefix(d.OpenAPI, "3.1") {
		t.Errorf("openapi version %q", d.OpenAPI)
	}
	if len(d.Servers) == 0 || d.Servers[0]["url"] != "/api/v1" {
		t.Errorf("servers: %v", d.Servers)
	}
	if v, _ := d.Info["version"].(string); !strings.HasPrefix(v, fmt.Sprint(apiVersion, ".", apiRevision)) {
		t.Errorf("info.version %q doesn't match API %d.%d", v, apiVersion, apiRevision)
	}
	raw, _ := os.ReadFile(openapiPath)
	for _, m := range regexp.MustCompile(`"#/components/schemas/([^"]+)"`).FindAllStringSubmatch(string(raw), -1) {
		if _, ok := d.Comps.Schemas[m[1]]; !ok {
			t.Errorf("reference to unknown schema %s", m[1])
		}
	}
	ops := map[string]bool{}
	for p, methods := range d.Paths {
		want := regexp.MustCompile(`\{([^}]+)\}`).FindAllStringSubmatch(p, -1)
		for method, raw := range methods {
			var op operation
			if err := json.Unmarshal(raw, &op); err != nil {
				t.Fatal(err)
			}
			where := method + " " + p
			if op.Summary == "" || op.Summary == "..." {
				t.Errorf("%s: no summary", where)
			}
			if len(op.Responses) == 0 {
				t.Errorf("%s: no responses", where)
			}
			if op.Auth != "public" && op.Auth != "user" && op.Auth != "admin" {
				t.Errorf("%s: x-wms-auth is %q", where, op.Auth)
			}
			if (op.Auth == "public") != (len(op.Security) == 0) {
				t.Errorf("%s: security doesn't match x-wms-auth", where)
			}
			if op.OperationI == "" || ops[op.OperationI] {
				t.Errorf("%s: operationId %q missing or used twice", where, op.OperationI)
			}
			ops[op.OperationI] = true
			for _, w := range want {
				found := false
				for _, pp := range op.Parameters {
					found = found || (pp.In == "path" && pp.Name == w[1])
				}
				if !found {
					t.Errorf("%s: path parameter %s isn't declared", where, w[1])
				}
			}
		}
	}
}

// ---------------------------------------------------------------- schemas

// schemaTypes are the Go types whose JSON the API sends and takes. Their
// schemas are generated from the types, so they can't drift: after changing a
// type, regenerate them with
//
//	WMS_UPDATE_OPENAPI=1 go test ./internal/server -run TestSchemasMatchGoTypes
func schemaTypes() map[string]reflect.Type {
	ty := func(v any) reflect.Type { return reflect.TypeOf(v) }
	return map[string]reflect.Type{
		"User": ty(auth.User{}), "UserView": ty(userView{}), "Access": ty(library.Access{}),
		"Library": ty(library.Library{}), "Item": ty(library.Item{}), "Progress": ty(library.Progress{}), "Show": ty(library.Show{}), "Season": ty(library.Season{}),
		"DiscInfo": ty(library.DiscInfo{}), "DiscLabels": ty(library.DiscLabels{}), "Album": ty(library.Album{}), "Artist": ty(library.Artist{}), "Network": ty(library.Network{}),
		"Podcast": ty(library.Podcast{}), "Annotation": ty(library.Annotation{}), "SearchHit": ty(library.SearchHit{}), "Agent": ty(library.Agent{}), "AgentResult": ty(library.AgentResult{}),
		"ArtCandidate": ty(artCandidate{}), "DiscTitleView": ty(discTitle{}), "DiscChapterView": ty(discChapter{}), "DiscMenuView": ty(discMenu{}),
		"PlayInfo": ty(playInfo{}), "Track": ty(trackInfo{}),
		"Presentation": ty(dvdsession.Presentation{}), "PresentationMedia": ty(dvdsession.Media{}), "DiscMenu": ty(dvdsession.Menu{}), "DiscButton": ty(dvdsession.ButtonView{}),
		"DiscTitlePlaying": ty(dvdsession.TitleView{}), "DiscTrack": ty(dvdsession.Track{}), "DiscAction": ty(dvdsession.Action{}), "SubEvent": ty(dvdsession.SubEvent{}),
		"PlaybackProfile": ty(playback.Profile{}), "PlaybackCapabilities": ty(playback.Capabilities{}), "PlaybackRequest": ty(playbackRequest{}), "PlaybackResponse": ty(playbackResponse{}),
		"PlaybackAudio": ty(playbackAudio{}), "PlaybackSubtitle": ty(playbackSubtitle{}), "PlaybackIntro": ty(playbackIntro{}), "PlaybackDisc": ty(playbackDisc{}),
		"ThemeMeta": ty(themes.Meta{}), "ThemeTokens": ty(themes.Tokens{}), "UpdateStatus": ty(update.Status{}), "UpdateInfo": ty(update.Info{}),
		"LogsView": ty(logsView{}), "ModuleInfo": ty(moduleInfo{}),
	}
}

type schemaGen struct {
	names map[reflect.Type]string
	t     *testing.T
}

func (g *schemaGen) of(ty reflect.Type) map[string]any {
	if n, ok := g.names[ty]; ok {
		return map[string]any{"$ref": "#/components/schemas/" + n}
	}
	switch ty.Kind() {
	case reflect.Pointer:
		return map[string]any{"anyOf": []any{g.of(ty.Elem()), map[string]any{"type": "null"}}}
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Slice, reflect.Array:
		return map[string]any{"type": []any{"array", "null"}, "items": g.of(ty.Elem())}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": g.of(ty.Elem())}
	case reflect.Struct:
		return g.object(ty)
	case reflect.Interface:
		return map[string]any{}
	}
	g.t.Fatalf("no schema for %s", ty)
	return nil
}

func (g *schemaGen) object(ty reflect.Type) map[string]any {
	props := map[string]any{}
	required := []any{}
	var walk func(reflect.Type)
	walk = func(st reflect.Type) {
		for i := range st.NumField() {
			f := st.Field(i)
			if !f.IsExported() {
				continue
			}
			tag := f.Tag.Get("json")
			name, opts, _ := strings.Cut(tag, ",")
			if name == "-" {
				continue
			}
			if f.Anonymous && name == "" {
				walk(f.Type)
				continue
			}
			if name == "" {
				name = f.Name
			}
			props[name] = g.of(f.Type)
			if !strings.Contains(opts, "omitempty") {
				required = append(required, name)
			}
		}
	}
	walk(ty)
	sort.Slice(required, func(i, j int) bool { return required[i].(string) < required[j].(string) })
	out := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

func canonical(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSchemasMatchGoTypes(t *testing.T) {
	types := schemaTypes()
	g := &schemaGen{names: map[reflect.Type]string{}, t: t}
	for name, ty := range types {
		g.names[ty] = name
	}
	generated := map[string]any{}
	for name, ty := range types {
		generated[name] = g.object(ty)
	}
	if os.Getenv("WMS_UPDATE_OPENAPI") == "1" {
		updateSchemas(t, generated)
	}
	d := readOpenAPI(t)
	for name, want := range generated {
		have, ok := d.Comps.Schemas[name]
		if !ok {
			t.Errorf("schema %s is missing from docs/openapi.json", name)
			continue
		}
		var got any
		_ = json.Unmarshal(have, &got)
		if canonical(t, got) != canonical(t, want) {
			t.Errorf("schema %s in docs/openapi.json doesn't match its Go type. Run: WMS_UPDATE_OPENAPI=1 go test ./internal/server -run TestSchemasMatchGoTypes", name)
		}
	}
}

// updateSchemas rewrites the generated part of the file, leaving the rest.
func updateSchemas(t *testing.T, generated map[string]any) {
	t.Helper()
	raw, err := os.ReadFile(openapiPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	comps, _ := doc["components"].(map[string]any)
	schemas, _ := comps["schemas"].(map[string]any)
	for name, s := range generated {
		schemas[name] = s
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(openapiPath, append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDocsNameNoOtherServers(t *testing.T) {
	banned := regexp.MustCompile(`(?i)\b(plex|jellyfin|emby|kodi|ampchor)\b`)
	for _, p := range []string{openapiPath, clientDoc} {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if m := banned.Find(b); m != nil {
			t.Errorf("%s names %q", p, m)
		}
	}
}
