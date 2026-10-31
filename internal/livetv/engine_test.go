package livetv

import (
	"context"
	"errors"
	"testing"

	"github.com/wittywilla/wittytuner"
)

// A running source comes from the engine in use, and settings are still checked by the
// built-in copy, so a version can't change what may be saved.
func TestEngineMakesSources(t *testing.T) {
	oldSource, oldDiscover := newSource, discoverTuners
	defer SetEngine(oldSource, oldDiscover)
	made, looked := 0, 0
	SetEngine(func(d string, c wittytuner.Config, e wittytuner.Env) (wittytuner.Source, error) {
		made++
		return nil, errors.New("from the engine")
	}, func(context.Context, wittytuner.Env) []wittytuner.Found {
		looked++
		return []wittytuner.Found{{Driver: "network", Name: "Found"}}
	})
	m := testManager(t)
	ctx := context.Background()
	if _, err := m.Create(ctx, &Tuner{Name: "Bad", Driver: "stream", Options: map[string]string{"url": "file:///etc/passwd"}}); err == nil || made != 0 {
		t.Fatalf("%v %d", err, made)
	}
	tu, err := m.Create(ctx, &Tuner{Name: "Good", Driver: "stream", Options: map[string]string{"url": "http://192.168.1.5/x"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Refresh(ctx, tu.ID); err == nil || err.Error() != "from the engine" || made == 0 {
		t.Fatalf("%v %d", err, made)
	}
	if f := m.Discover(ctx); len(f) != 1 || f[0].Name != "Found" || looked != 1 {
		t.Fatalf("%+v", f)
	}
}
