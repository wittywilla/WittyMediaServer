package stream

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/wittywilla/wittyplay/ipc"
)

// These run the real runner program with its test core. No encoder is needed, because
// nobody is watching: a state can be saved and loaded from a paused game.

var (
	controlRunner     string
	controlRunnerOnce sync.Once
)

// The runner is built once here: every test in this file only needs the same program.
func controlManager(t *testing.T) *Manager {
	t.Helper()
	controlRunnerOnce.Do(func() {
		dir, err := os.MkdirTemp("", "wittyplay-control")
		if err != nil {
			t.Fatal(err)
		}
		controlRunner = filepath.Join(dir, "wittyplay-host")
		if runtime.GOOS == "windows" {
			controlRunner += ".exe"
		}
		cmd := exec.Command("go", "build", "-o", controlRunner, "../cmd/wittyplay-host")
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("building the host: %v\n%s", err, b)
		}
	})
	return NewManager(Config{Runner: controlRunner, FFmpeg: "ffmpeg", HaveEncoder: func(string) bool { return false }})
}

func controlGame(t *testing.T, m *Manager, save string, o Options) *Session {
	t.Helper()
	rom := filepath.Join(t.TempDir(), "game.wittytest")
	if err := os.WriteFile(rom, []byte("a test game"), 0o644); err != nil {
		t.Fatal(err)
	}
	o.UserID, o.ItemID, o.Title, o.Core, o.ROM, o.SystemDir, o.SaveDir = 1, 7, "Test", ipc.TestCore, rom, filepath.Dir(rom), save
	s, err := m.Start(o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Stop)
	return s
}

func stateTick(t *testing.T, path string) uint32 {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return binary.LittleEndian.Uint32(b[4:])
}

func TestSlotNames(t *testing.T) {
	seen := map[string]bool{}
	for _, slot := range append([]int{0, SlotUndo, SlotPrevAuto}, 1, MaxSlot) {
		name, err := SlotName(slot)
		if err != nil {
			t.Errorf("slot %d: %v", slot, err)
		}
		if seen[name] {
			t.Errorf("slot %d shares the name %s", slot, name)
		}
		seen[name] = true
	}
	for _, bad := range []int{-1, SlotPrevAuto + 1, 1 << 40} {
		if name, err := SlotName(bad); err == nil {
			t.Errorf("slot %d became %s", bad, name)
		}
	}
}

func TestAnUndoFollowsALoadAndGoesBothWays(t *testing.T) {
	m := controlManager(t)
	dir := t.TempDir()
	s := controlGame(t, m, dir, Options{})
	ctx := context.Background()
	if err := s.SaveState(ctx, 5); err != nil {
		t.Fatal(err)
	}
	if err := s.Undo(ctx); err == nil {
		t.Fatal("there was nothing to undo, but it said there was")
	}
	if err := s.LoadState(5); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "undo.state")); err != nil {
		t.Fatalf("a load kept no undo: %v", err)
	}
	if err := s.Undo(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.LoadState(SlotUndo); err != nil {
		t.Fatalf("loading the undo slot: %v", err)
	}
}

func TestStartingOverKeepsTheOldAutomaticSave(t *testing.T) {
	m := controlManager(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "auto.state"), []byte("the old one"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := controlGame(t, m, dir, Options{}) // not resuming
	s.Stop()
	b, err := os.ReadFile(filepath.Join(dir, "autoprev.state"))
	if err != nil || string(b) != "the old one" {
		t.Fatalf("the old save wasn't kept: %q %v", b, err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "auto.state")); string(b) == "the old one" {
		t.Fatal("the new automatic save wasn't written")
	}
}

func TestResumingDoesntDisturbTheOldSave(t *testing.T) {
	m := controlManager(t)
	dir := t.TempDir()
	first := controlGame(t, m, dir, Options{})
	first.Stop()
	before, _ := os.ReadFile(filepath.Join(dir, "autoprev.state"))
	s := controlGame(t, m, dir, Options{Resume: true})
	s.Stop()
	after, _ := os.ReadFile(filepath.Join(dir, "autoprev.state"))
	if string(before) != string(after) {
		t.Fatal("resuming replaced what starting over had kept")
	}
}

func TestAutoSaveCanBeOff(t *testing.T) {
	m := controlManager(t)
	dir := t.TempDir()
	s := controlGame(t, m, dir, Options{NoAutoSave: true})
	s.Stop()
	if _, err := os.Stat(filepath.Join(dir, "auto.state")); err == nil {
		t.Fatal("it saved anyway")
	}
	// And it can change while the game runs.
	s = controlGame(t, m, dir, Options{NoAutoSave: true})
	s.SetAutoSave(true)
	s.Stop()
	if _, err := os.Stat(filepath.Join(dir, "auto.state")); err != nil {
		t.Fatal("the change wasn't heard")
	}
}

func TestASaveOverItsLimitIsRefused(t *testing.T) {
	m := controlManager(t)
	dir := t.TempDir()
	s := controlGame(t, m, dir, Options{})
	f, err := os.Create(filepath.Join(dir, "big.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(MaxSaveBytes); err != nil {
		t.Skipf("no sparse file here: %v", err)
	}
	f.Close()
	if err := s.SaveState(context.Background(), 1); err != ErrSaveFull {
		t.Fatalf("got %v", err)
	}
	_ = os.Remove(filepath.Join(dir, "big.bin"))
}

func TestSpeedIsHeldToTheLimitGiven(t *testing.T) {
	m := controlManager(t)
	s := controlGame(t, m, t.TempDir(), Options{MaxSpeed: 3})
	for _, c := range []struct{ ask, want float64 }{{100, 3}, {2, 2}, {1, 1}, {0.0001, ipc.SpeedMin}} {
		got, err := s.SetSpeed(c.ask)
		if err != nil || got != c.want {
			t.Errorf("asked for %v, got %v (%v)", c.ask, got, err)
		}
	}
	for _, bad := range []float64{0, -1} {
		if _, err := s.SetSpeed(bad); err == nil {
			t.Errorf("%v was accepted as a speed", bad)
		}
	}
	// A session given no limit runs at normal speed and no faster.
	s2 := controlGame(t, m, t.TempDir(), Options{UserID: 2})
	if got, _ := s2.SetSpeed(8); got != 1 {
		t.Errorf("no limit given, yet %v", got)
	}
}

func TestRewindNeedsTurningOnFirst(t *testing.T) {
	m := controlManager(t)
	s := controlGame(t, m, t.TempDir(), Options{})
	if err := s.SetRewinding(true); err == nil {
		t.Fatal("it should say rewind is off")
	}
	on := int64(1 << 20)
	s2 := controlGame(t, m, t.TempDir(), Options{Control: &ipc.Control{RewindBytes: &on}})
	if err := s2.SetRewinding(true); err != nil {
		t.Fatal(err)
	}
}

func TestCoreOptionsArriveAndStartValuesTakeEffect(t *testing.T) {
	m := controlManager(t)
	got := make(chan []ipc.Option, 1)
	s := controlGame(t, m, t.TempDir(), Options{CoreOptions: map[string]string{"test_colours": "inverted", "test_speed": "../../etc/passwd", "no_such": "x"}, OnCoreOptions: func(l []ipc.Option) { got <- l }})
	select {
	case <-got:
	case <-time.After(5 * time.Second):
		t.Fatal("the core's list never came")
	}
	opts := map[string]string{}
	for _, o := range s.CoreOptions() {
		opts[o.Key] = o.Value
	}
	if opts["test_colours"] != "inverted" {
		t.Errorf("a good value wasn't taken: %v", opts)
	}
	if opts["test_speed"] != "2" {
		t.Errorf("a value it doesn't list was taken: %v", opts)
	}
	if _, ok := opts["no_such"]; ok {
		t.Error("an option it doesn't offer appeared")
	}
	if err := s.SetOption("test_speed", "4"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetOption("test_speed", "5"); err == nil {
		t.Error("a value it doesn't list wasn't refused")
	}
	if err := s.SetOption("no_such", "1"); err == nil {
		t.Error("an option it doesn't offer wasn't refused")
	}
	for _, o := range s.CoreOptions() {
		if o.Key == "test_colours" && (o.Category != "Picture" || o.Info == "" || o.Default != "normal" || o.Labels[1] != "Inverted") {
			t.Errorf("what the core said about it didn't arrive: %+v", o)
		}
	}
	// Settings that say nothing about an option put it back to the core's own default.
	if err := s.Apply(ipc.Control{}, map[string]string{"test_speed": "4"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, o := range s.CoreOptions() {
			opts[o.Key] = o.Value
		}
		if opts["test_colours"] == "normal" && opts["test_speed"] == "4" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("after applying: %v", opts)
}

func TestSafeModeTakesNoSettings(t *testing.T) {
	m := controlManager(t)
	on := int64(1 << 20)
	s := controlGame(t, m, t.TempDir(), Options{SafeMode: true, Control: &ipc.Control{RewindBytes: &on}, Cheats: []ipc.Cheat{{Code: "x=1", Enabled: true}}})
	if !s.Safe() || !s.Info().SafeMode {
		t.Fatal("it doesn't say it's in safe mode")
	}
	if err := s.Apply(ipc.Control{RewindBytes: &on}, map[string]string{"test_colours": "inverted"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	for _, o := range s.CoreOptions() {
		if o.Key == "test_colours" && o.Value != "normal" {
			t.Fatal("a setting reached a safe-mode game")
		}
	}
}

func TestProbeListsWithoutAGame(t *testing.T) {
	m := controlManager(t)
	list, err := m.ProbeOptions(context.Background(), ipc.TestCore, t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Key != "test_colours" {
		t.Fatalf("got %+v", list)
	}
}

func TestWhatARunnerSaysAboutItsOptionsIsCutToSize(t *testing.T) {
	long := make([]byte, 5000)
	for i := range long {
		long[i] = 'a'
	}
	big := make([]ipc.Option, maxOffered+50)
	for i := range big {
		big[i] = ipc.Option{Key: "k", Choices: []string{"a"}}
	}
	if got := cleanOffered(big); len(got) > maxOffered {
		t.Errorf("kept %d", len(got))
	}
	got := cleanOffered([]ipc.Option{
		{Key: "", Choices: []string{"a"}},
		{Key: string(long), Choices: []string{"a"}},
		{Key: "nothing", Choices: nil},
		{Key: "long-choice", Choices: []string{string(long)}},
		{Key: "ok", Label: string(long), Choices: []string{"a", string(long)}, Value: string(long)},
	})
	if len(got) != 1 || got[0].Key != "ok" || len(got[0].Label) > maxOfferedText || len(got[0].Choices) != 1 || len(got[0].Value) > ipc.MaxOptionValue {
		b, _ := json.Marshal(got)
		t.Fatalf("got %d: %.200s", len(got), b)
	}
}

func TestStatsWithNobodyWatching(t *testing.T) {
	m := controlManager(t)
	s := controlGame(t, m, t.TempDir(), Options{MaxSpeed: 2})
	st := s.Stats()
	if st.Core == "" || st.Speed != 1 || !st.Paused || st.GameFPS != 0 {
		t.Fatalf("%+v", st)
	}
	if s.Info().MaxSpeed != 2 {
		t.Fatalf("max speed: %v", s.Info().MaxSpeed)
	}
}

func TestForUserFindsOnlyThatPersonsGame(t *testing.T) {
	m := controlManager(t)
	s := controlGame(t, m, t.TempDir(), Options{})
	if m.ForUser(1) != s {
		t.Fatal("didn't find it")
	}
	if m.ForUser(2) != nil {
		t.Fatal("found someone else's")
	}
}
