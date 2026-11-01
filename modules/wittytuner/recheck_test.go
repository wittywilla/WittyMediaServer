package wittytuner

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const clearMedia = "#EXTM3U\n#EXT-X-TARGETDURATION:6\n#EXTINF:6,\nseg.ts\n"
const lockedMedia = "#EXTM3U\n#EXT-X-TARGETDURATION:6\n#EXT-X-KEY:METHOD=SAMPLE-AES,URI=\"key\"\n#EXTINF:6,\nseg.ts\n"

// A live playlist that adds a key partway through, as an opening check can't see.
func TestRecheckFindsAKeyThatComesLater(t *testing.T) {
	for name, master := range map[string]string{
		"a media playlist": "",
		"behind a master":  "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\nmedia.m3u8\n",
	} {
		var locked atomic.Bool
		mux := http.NewServeMux()
		media := func(w http.ResponseWriter, r *http.Request) {
			if locked.Load() {
				fmt.Fprint(w, lockedMedia)
				return
			}
			fmt.Fprint(w, clearMedia)
		}
		mux.HandleFunc("/media.m3u8", media)
		mux.HandleFunc("/live.m3u8", func(w http.ResponseWriter, r *http.Request) {
			if master == "" {
				media(w, r)
				return
			}
			fmt.Fprint(w, master)
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		st, err := openStream(t, srv.URL+"/live.m3u8")
		if err != nil || st.Recheck == nil || !st.Verified {
			t.Fatalf("%s: %v %+v", name, err, st)
		}
		why, next, err := st.Recheck(context.Background())
		if why != "" || err != nil {
			t.Fatalf("%s: clear so far: %q %v", name, why, err)
		}
		if next != 6*time.Second {
			t.Errorf("%s: it should go by the playlist's own target duration: %v", name, next)
		}
		locked.Store(true)
		why, _, err = st.Recheck(context.Background())
		if err != nil || !strings.Contains(why, "DRM") {
			t.Errorf("%s: the key went unnoticed: %q %v", name, why, err)
		}
	}
}

// A failure to find out says nothing: a clear stream goes on, and a key still counts
// when it comes with an error.
func TestRecheckThroughFailures(t *testing.T) {
	var mode atomic.Value
	mode.Store("clear")
	mux := http.NewServeMux()
	mux.HandleFunc("/live.m3u8", func(w http.ResponseWriter, r *http.Request) {
		switch mode.Load() {
		case "down":
			w.WriteHeader(503)
		case "key with an error":
			w.WriteHeader(500)
			fmt.Fprint(w, lockedMedia)
		default:
			fmt.Fprint(w, clearMedia)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	st, err := openStream(t, srv.URL+"/live.m3u8")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if why, _, err := st.Recheck(ctx); why != "" || err != nil {
		t.Fatalf("%q %v", why, err)
	}
	mode.Store("down")
	why, next, err := st.Recheck(ctx)
	if why != "" || err == nil {
		t.Errorf("a failure: %q %v", why, err)
	}
	if next < hlsRecheckMin {
		t.Errorf("it should still say when to try again: %v", next)
	}
	mode.Store("key with an error")
	if why, _, _ := st.Recheck(ctx); why == "" {
		t.Error("a key came back and went unnoticed")
	}
}

// How often it looks, on the smallest of machines: never much more than its floor, and
// a master of many variants has only a few read each time, but all of them in turn.
func TestRecheckIsLight(t *testing.T) {
	for _, c := range []struct {
		target string
		want   time.Duration
	}{{"1", hlsRecheckMin}, {"6", 6 * time.Second}, {"600", hlsRecheckMax}, {"nonsense", hlsRecheckMin}} {
		mux := http.NewServeMux()
		mux.HandleFunc("/live.m3u8", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "#EXTM3U\n#EXT-X-TARGETDURATION:%s\n#EXTINF:6,\nseg.ts\n", c.target)
		})
		srv := httptest.NewServer(mux)
		st, err := openStream(t, srv.URL+"/live.m3u8")
		if err != nil {
			t.Fatal(err)
		}
		if _, next, err := st.Recheck(context.Background()); next != c.want || err != nil {
			t.Errorf("target %s: %v %v", c.target, next, err)
		}
		srv.Close()
	}

	const variants = 10
	var asked [variants]atomic.Int64
	var master strings.Builder
	master.WriteString("#EXTM3U\n")
	mux := http.NewServeMux()
	for i := range variants {
		fmt.Fprintf(&master, "#EXT-X-STREAM-INF:BANDWIDTH=1\nv%d.m3u8\n", i)
		mux.HandleFunc(fmt.Sprintf("/v%d.m3u8", i), func(w http.ResponseWriter, r *http.Request) {
			asked[i].Add(1)
			fmt.Fprint(w, clearMedia)
		})
	}
	mux.HandleFunc("/live.m3u8", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, master.String()) })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	st, err := openStream(t, srv.URL+"/live.m3u8")
	if err != nil {
		t.Fatal(err)
	}
	for i := range variants {
		asked[i].Store(0)
	}
	if _, _, err := st.Recheck(context.Background()); err != nil {
		t.Fatal(err)
	}
	read := 0
	for i := range variants {
		read += int(asked[i].Load())
	}
	if read != hlsVariantsPerCheck {
		t.Errorf("%d variants were read in one check", read)
	}
	for range variants {
		if _, _, err := st.Recheck(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	for i := range variants {
		if asked[i].Load() == 0 {
			t.Errorf("variant %d was never looked at", i)
		}
	}
}
