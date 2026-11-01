package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/wittywilla/wittyplay/stream"
)

// A core is a program, and reads any file the server can. Knowing the password is not enough to
// run one: someone with a stolen session could make an administrator with a password of their own.
func TestCustomCoresNeedTheMachinesSay(t *testing.T) {
	ts := newTestServer(t)
	_, tok := ts.user(t, "owner", "adult", true, "")
	enable := map[string]any{"enabled": true, "password": "correct-horse-1"}
	if w := ts.do(t, "PUT", "/api/gaming/custom/enabled", tok, enable); w.Code != 403 {
		t.Fatalf("turned on from a web session alone: %d %s", w.Code, w.Body)
	}
	// A server that had them on before keeps them off until the machine's owner says so.
	if err := ts.DB.SetSetting(context.Background(), settingCustomCores, "1"); err != nil {
		t.Fatal(err)
	}
	if ts.customCoresOn(context.Background()) {
		t.Fatal("a saved setting alone turned them on")
	}
	w := ts.do(t, "GET", "/api/gaming/custom", tok, nil)
	var st map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st["enabled"] != false || st["canEnable"] != false || st["needsSwitch"] != true {
		t.Fatalf("%v", st)
	}

	// Allowed on the machine, the setting counts again. The lock beats the switch.
	ts.Cfg.GamingCustomCores = true
	if _, sandboxed := stream.SandboxStrength(); sandboxed && !ts.customCoresOn(context.Background()) {
		t.Error("allowed on the machine and turned on in Settings")
	}
	t.Setenv(lockCustomCores, "1")
	if ts.customCoresOn(context.Background()) {
		t.Error("the lock must beat the switch")
	}
	if w := ts.do(t, "PUT", "/api/gaming/custom/enabled", tok, enable); w.Code != 403 {
		t.Errorf("locked, turned on: %d", w.Code)
	}
}
