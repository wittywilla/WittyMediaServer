package livetv

import (
	"context"
	"errors"
	"strings"
	"testing"

	wittytuner "github.com/wittywilla/wittymediaserver/internal/tuner"
)

// fakeEngine is WittyTuner, as a test says it behaves.
type fakeEngine struct {
	made, looked int
}

func (f *fakeEngine) New(_ context.Context, driver string, c wittytuner.Config, _ wittytuner.Env) (wittytuner.Source, error) {
	f.made++
	if strings.HasPrefix(c.Options["url"], "file:") {
		return nil, errors.New("that's not an address for this")
	}
	return nil, errors.New("from the engine")
}

func (f *fakeEngine) Discover(context.Context, wittytuner.Env) ([]wittytuner.Found, error) {
	f.looked++
	return []wittytuner.Found{{Driver: "network", Name: "Found"}}, nil
}

func (f *fakeEngine) Drivers(context.Context, wittytuner.Env) ([]wittytuner.Driver, error) {
	return nil, nil
}

func (f *fakeEngine) SecretFields(context.Context, string, wittytuner.Env) ([]string, error) {
	return []string{"password"}, nil
}

// A running source comes from the engine in use, and so does what settings it takes.
func TestEngineMakesSources(t *testing.T) {
	old := engine
	defer SetEngine(old)
	f := &fakeEngine{}
	SetEngine(f)
	m := testManager(t)
	ctx := context.Background()
	if _, err := m.Create(ctx, &Tuner{Name: "Bad", Driver: "stream", Options: map[string]string{"url": "file:///etc/passwd"}}); err == nil || f.made == 0 {
		t.Fatalf("%v %d", err, f.made)
	}
	if found, err := m.Discover(ctx); err != nil || len(found) != 1 || found[0].Name != "Found" || f.looked != 1 {
		t.Fatalf("%+v %v", found, err)
	}
}

func TestNoEngineSaysSo(t *testing.T) {
	old := engine
	defer SetEngine(old)
	SetEngine(nil)
	m := testManager(t)
	if _, err := m.Discover(context.Background()); !errors.Is(err, ErrNoEngine) {
		t.Fatalf("%v", err)
	}
	if _, err := m.Create(context.Background(), &Tuner{Name: "Any", Driver: "stream"}); !errors.Is(err, ErrNoEngine) {
		t.Fatalf("%v", err)
	}
}
