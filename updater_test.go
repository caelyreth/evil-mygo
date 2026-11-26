//go:build !js

package mygo

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/update"
	"github.com/egoist/mygo/internal/wait"
)

func testTarget() string { return Updater.Target() }

// updateServer serves a signed update whose app holds main with content,
// and with deltaFrom, a delta of it from that version of installedApp.
func updateServer(t *testing.T, version, content, deltaFrom string) (srv *httptest.Server, key string, m update.Manifest, requests func(string) int) {
	t.Helper()
	pub, priv, _ := ed25519.GenerateKey(nil)
	src := t.TempDir()
	app := "app"
	if runtime.GOOS == "darwin" {
		app = "Test.app"
	}
	main := filepath.Join(src, app, "main")
	if err := os.MkdirAll(filepath.Dir(main), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(main, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, app, "resource"), []byte("unchanged"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, app, "code"), newCode, 0o644); err != nil {
		t.Fatal(err)
	}
	entries := []string{app}
	if runtime.GOOS != "darwin" {
		src, entries = filepath.Join(src, app), []string{"code", "main", "resource"}
	}
	var archive bytes.Buffer
	if err := update.WriteArchive(&archive, src, entries); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(archive.Bytes())
	var mu sync.Mutex
	counts := map[string]int{}
	mux := http.NewServeMux()
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		counts[r.URL.Path]++
		mu.Unlock()
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)

	m = update.Manifest{
		Schema:  update.SchemaVersion,
		Target:  testTarget(),
		Channel: update.DefaultChannel,
		Version: version,
		Notes:   "- Faster",
		NotesByLang: map[string]string{
			"ja": "- 高速化",
		},
		Date: "2026-09-27T10:00:00Z",
		Artifact: update.Artifact{
			URL:       srv.URL + "/app.tar.zst",
			Size:      int64(archive.Len()),
			SHA256:    hex.EncodeToString(sum[:]),
			Signature: update.SignArtifact(priv, sum[:]),
		},
	}
	mux.HandleFunc("/app.tar.zst", func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "app.tar.zst", time.Time{}, bytes.NewReader(archive.Bytes()))
	})
	if deltaFrom != "" {
		old, _ := installedApp(t)
		var delta bytes.Buffer
		if err := update.WriteDelta(&delta, deltaFrom, version, old, filepath.Join(src, entries[0]), nil); runtime.GOOS != "darwin" {
			delta.Reset()
			err = update.WriteDelta(&delta, deltaFrom, version, old, src, entries)
		} else if err != nil {
			t.Fatal(err)
		}
		dSum := sha256.Sum256(delta.Bytes())
		m.Deltas = []update.Delta{{
			From:      deltaFrom,
			URL:       srv.URL + "/app.delta",
			Size:      int64(delta.Len()),
			SHA256:    hex.EncodeToString(dSum[:]),
			Signature: update.SignArtifact(priv, dSum[:]),
		}}
		mux.HandleFunc("/app.delta", func(w http.ResponseWriter, r *http.Request) {
			http.ServeContent(w, r, "app.delta", time.Time{}, bytes.NewReader(delta.Bytes()))
		})
	}
	signed, err := update.SignManifest([]ed25519.PrivateKey{priv}, &m)
	if err != nil {
		t.Fatal(err)
	}
	mux.HandleFunc("/update.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(signed)
	})
	return srv, base64.StdEncoding.EncodeToString(pub), m, func(path string) int {
		mu.Lock()
		defer mu.Unlock()
		return counts[path]
	}
}

func installedApp(t *testing.T) (target, main string) {
	t.Helper()
	dir := t.TempDir()
	target = dir
	if runtime.GOOS == "darwin" {
		target = filepath.Join(dir, "My App.app")
	}
	main = filepath.Join(target, "main")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(main, []byte("v1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "resource"), []byte("unchanged"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "code"), oldCode, 0o644); err != nil {
		t.Fatal(err)
	}
	return target, main
}

var oldCode, newCode = func() ([]byte, []byte) {
	r := rand.New(rand.NewPCG(1, 2))
	old := make([]byte, 64<<10)
	for i := range old {
		old[i] = byte(r.Uint32())
	}
	new := bytes.Clone(old)
	for range 20 {
		new[r.IntN(len(new))]++
	}
	return old, new
}()

func useIsolatedCache(t *testing.T) {
	t.Helper()
	prev := App.paths[PathCache]
	dir := t.TempDir()
	App.SetPath(PathCache, dir)
	t.Cleanup(func() { App.SetPath(PathCache, prev) })
}

func TestUpdater(t *testing.T) {
	useIsolatedCache(t)
	ctx := context.Background()
	srv, key, m, _ := updateServer(t, "1.2.0", "v2", "")

	if _, err := Updater.Check(ctx); err != ErrUpdatesDisabled {
		t.Errorf("Check without updates: %v", err)
	}
	up, err := checkUpdate(ctx, feedSource(srv.URL+"/update.json"), key, testTarget(), "stable", "1.1.9", "")
	if err != nil || up == nil || up.Version != "1.2.0" || up.Notes != "- Faster" || up.Date.Year() != 2026 {
		t.Fatalf("checkUpdate = %+v, %v", up, err)
	}
	if got := up.NotesFor("ja"); got != "- 高速化" {
		t.Errorf("NotesFor(ja) = %q", got)
	}
	for _, current := range []string{"1.2.0", "1.10.0", "1.3.0-beta.1"} {
		if up, err := checkUpdate(ctx, feedSource(srv.URL+"/update.json"), key, testTarget(), "stable", current, ""); up != nil || err != nil {
			t.Errorf("checkUpdate from %s = %+v, %v", current, up, err)
		}
	}
	if up, err := checkUpdate(ctx, feedSource(srv.URL+"/update.json"), key, testTarget(), "stable", "1.2.0-rc.1", ""); err != nil || up == nil || up.Version != "1.2.0" {
		t.Errorf("checkUpdate from 1.2.0-rc.1 = %+v, %v", up, err)
	}
	if _, err := checkUpdate(ctx, feedSource("http://example.com/update.json"), key, testTarget(), "stable", "1.0.0", ""); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Errorf("plain HTTP: %v", err)
	}

	target, main := installedApp(t)
	quit := runningApp(t)
	var progress [2]int64
	err = installUpdate(ctx, httpGetRange, up.manifest, up.keyring, up.current, target, func(n, total int64) { progress = [2]int64{n, total} })
	if err != nil {
		t.Fatal(err)
	}
	quit()
	if b, _ := os.ReadFile(main); string(b) != "v2" {
		t.Errorf("after the update main = %q", b)
	}
	if progress != [2]int64{m.Artifact.Size, m.Artifact.Size} {
		t.Errorf("progress = %v, want %d", progress, m.Artifact.Size)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(target), ".*")); runtime.GOOS == "darwin" && len(leftovers) > 0 {
		t.Errorf("left behind: %q", leftovers)
	}

	// Refused manifest / corrupted digest / wrong key changes nothing.
	target, main = installedApp(t)
	otherKey, _, _ := ed25519.GenerateKey(nil)
	if _, err := checkUpdate(ctx, feedSource(srv.URL+"/update.json"), base64.StdEncoding.EncodeToString(otherKey), testTarget(), "stable", "1.0.0", ""); err == nil {
		t.Error("manifest verified under wrong key")
	}
	bad := up.manifest
	bad.Artifact.SHA256 = strings.Repeat("0", 64)
	if err := installUpdate(ctx, httpGetRange, bad, up.keyring, up.current, target, nil); err == nil {
		t.Errorf("bad sha256 succeeded")
	}
	short := up.manifest
	short.Artifact.Size++
	if err := installUpdate(ctx, httpGetRange, short, up.keyring, up.current, target, nil); err == nil {
		t.Errorf("wrong size succeeded")
	}
	if b, _ := os.ReadFile(main); string(b) != "v1" {
		t.Errorf("a refused update changed main to %q", b)
	}
}

func TestUpdaterMultiMirrorAndRangeResume(t *testing.T) {
	useIsolatedCache(t)
	oldWait := mirrorRetryWait
	mirrorRetryWait = func(context.Context, time.Duration) bool { return true }
	t.Cleanup(func() { mirrorRetryWait = oldWait })

	payload := bytes.Repeat([]byte("neverland-mirror-data-"), 4096)
	sum := sha256.Sum256(payload)
	hexSum := hex.EncodeToString(sum[:])

	var m1Count, m2Count atomic.Int32
	var m2RangeHeader string
	// Mirror 1 writes only first 10_000 bytes and aborts mid-stream.
	m1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m1Count.Add(1)
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		w.Write(payload[:10_000])
		w.(http.Flusher).Flush()
		panic(http.ErrAbortHandler)
	}))
	defer m1.Close()

	// Mirror 2 supports Range and serves the remainder.
	m2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m2Count.Add(1)
		if rng := r.Header.Get("Range"); rng != "" {
			m2RangeHeader = rng
		}
		http.ServeContent(w, r, "app.tar.zst", time.Time{}, bytes.NewReader(payload))
	}))
	defer m2.Close()

	pub, priv, _ := ed25519.GenerateKey(nil)
	ring, _ := update.ParseKeyring(base64.StdEncoding.EncodeToString(pub))
	sig := update.SignArtifact(priv, sum[:])
	dst := filepath.Join(t.TempDir(), "downloaded.tar.zst")

	var last [2]int64
	err := downloadMirrors(t.Context(), httpGetRange, []string{m1.URL + "/a.tar.zst", m2.URL + "/a.tar.zst"},
		int64(len(payload)), hexSum, sig, ring, testTarget(), dst, func(d, total int64) { last = [2]int64{d, total} })
	if err != nil {
		t.Fatal(err)
	}
	if m1Count.Load() != 1 || m2Count.Load() != 1 {
		t.Fatalf("mirror counts m1=%d m2=%d", m1Count.Load(), m2Count.Load())
	}
	if m2RangeHeader != "bytes=10000-" {
		t.Errorf("mirror 2 Range = %q, want bytes=10000-", m2RangeHeader)
	}
	if last != [2]int64{int64(len(payload)), int64(len(payload))} {
		t.Errorf("progress = %v", last)
	}
	got, _ := os.ReadFile(dst)
	if !bytes.Equal(got, payload) {
		t.Error("resumed content mismatch")
	}
}

func TestUpdaterETagAndKeyRotation(t *testing.T) {
	useIsolatedCache(t)
	pub1, priv1, _ := ed25519.GenerateKey(nil)
	pub2, priv2, _ := ed25519.GenerateKey(nil)
	m := update.Manifest{
		Target:  testTarget(),
		Channel: "stable",
		Version: "2.0.0",
		Artifact: update.Artifact{
			URL:    "https://dl.example.com/2.0.0/app.tar.zst",
			Size:   10,
			SHA256: strings.Repeat("a", 64),
		},
	}
	// Signed by rotated key2; client trusts both key1 and key2.
	signed, err := update.SignManifest([]ed25519.PrivateKey{priv2}, &m)
	if err != nil {
		t.Fatal(err)
	}
	_ = priv1
	ringRaw := update.EncodeKeyring([]ed25519.PublicKey{pub1, pub2})

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("If-None-Match") == `"v2-etag"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v2-etag"`)
		w.Write(signed)
	}))
	defer srv.Close()

	src := feedSource(srv.URL + "/update.json")
	up1, err := checkUpdateCached(t.Context(), src, srv.URL, ringRaw, testTarget(), "stable", "1.0.0", "en")
	if err != nil || up1 == nil || up1.Version != "2.0.0" {
		t.Fatalf("first check = %+v, %v", up1, err)
	}
	up2, err := checkUpdateCached(t.Context(), src, srv.URL, ringRaw, testTarget(), "stable", "1.0.0", "en")
	if err != nil || up2 == nil || up2.Version != "2.0.0" {
		t.Fatalf("304 check = %+v, %v", up2, err)
	}
	if calls.Load() != 2 {
		t.Errorf("server calls = %d", calls.Load())
	}
}

func TestUpdaterDelta(t *testing.T) {
	useIsolatedCache(t)
	ctx := context.Background()
	srv, key, m, requests := updateServer(t, "1.2.0", "v2, a bigger version", "1.1.9")
	up, err := checkUpdate(ctx, feedSource(srv.URL+"/update.json"), key, testTarget(), "stable", "1.1.9", "")
	if err != nil || len(up.manifest.Deltas) != 1 {
		t.Fatalf("checkUpdate = %+v, %v", up, err)
	}
	install := func(m update.Manifest, current string, corrupt bool) (progress [][2]int64) {
		t.Helper()
		target, main := installedApp(t)
		quit := runningApp(t)
		if corrupt {
			os.WriteFile(filepath.Join(target, "code"), newCode[:1000], 0o644)
		}
		err := installUpdate(ctx, httpGetRange, m, up.keyring, current, target, func(n, total int64) { progress = append(progress, [2]int64{n, total}) })
		if err != nil {
			t.Fatal(err)
		}
		quit()
		if b, _ := os.ReadFile(main); string(b) != "v2, a bigger version" {
			t.Errorf("after the update main = %q", b)
		}
		if b, _ := os.ReadFile(filepath.Join(target, "resource")); string(b) != "unchanged" {
			t.Errorf("after the update resource = %q", b)
		}
		if b, _ := os.ReadFile(filepath.Join(target, "code")); !bytes.Equal(b, newCode) {
			t.Error("after the update code is not the new one")
		}
		return progress
	}
	delta := m.Deltas[0]

	if delta.Size > m.Artifact.Size/10 {
		t.Errorf("the delta takes %d bytes, the archive %d", delta.Size, m.Artifact.Size)
	}
	if progress := install(up.manifest, up.current, false); progress[len(progress)-1] != [2]int64{delta.Size, delta.Size} {
		t.Errorf("progress = %v, want the delta's %d bytes", progress, delta.Size)
	}
	if requests("/app.delta") != 1 || requests("/app.tar.zst") != 0 {
		t.Errorf("downloaded the delta %d times and the archive %d", requests("/app.delta"), requests("/app.tar.zst"))
	}

	if progress := install(up.manifest, up.current, true); progress[len(progress)-1] != [2]int64{m.Artifact.Size, m.Artifact.Size} {
		t.Errorf("progress = %v, want the archive's %d bytes last", progress, m.Artifact.Size)
	}
	if requests("/app.delta") != 2 || requests("/app.tar.zst") != 1 {
		t.Errorf("downloaded the delta %d times and the archive %d", requests("/app.delta"), requests("/app.tar.zst"))
	}
	install(up.manifest, "1.1.8", false)
	if requests("/app.delta") != 2 || requests("/app.tar.zst") != 2 {
		t.Errorf("downloaded the delta %d times and the archive %d", requests("/app.delta"), requests("/app.tar.zst"))
	}
}

func runningApp(t *testing.T) (quit func()) {
	t.Helper()
	if runtime.GOOS != "darwin" {
		return func() {}
	}
	app := exec.Command("/bin/sleep", "60")
	if err := app.Start(); err != nil {
		t.Fatal(err)
	}
	old := appPid
	appPid = app.Process.Pid
	t.Cleanup(func() { appPid = old })
	return func() {
		t.Helper()
		app.Process.Kill()
		app.Wait()
		p := pendingUpdate.Swap(nil)
		if p == nil {
			t.Fatal("no update staged")
		}
		if err := p.helper.Wait(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestUpdaterWaitsForTheAppToExit(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("only macOS waits for the app to exit")
	}
	useIsolatedCache(t)
	ctx := context.Background()
	srv, key, _, _ := updateServer(t, "1.2.0", "v2", "")
	up, err := checkUpdate(ctx, feedSource(srv.URL+"/update.json"), key, testTarget(), "stable", "1.1.9", "")
	if err != nil {
		t.Fatal(err)
	}
	target, main := installedApp(t)
	quit := runningApp(t)
	if err := installUpdate(ctx, httpGetRange, up.manifest, up.keyring, up.current, target, nil); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	if b, _ := os.ReadFile(main); string(b) != "v1" {
		t.Errorf("while the app runs main = %q", b)
	}
	quit()
	if b, _ := os.ReadFile(main); string(b) != "v2" {
		t.Errorf("after the app exited main = %q", b)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(target), ".*")); len(leftovers) > 0 {
		t.Errorf("left behind: %q", leftovers)
	}
}

func TestUpdaterChannelAndPrereleases(t *testing.T) {
	t.Cleanup(func() { Updater.SetChannel("") })
	if Updater.Channel() != update.DefaultChannel || Updater.Prereleases() {
		t.Error("default channel should be stable")
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 100 {
			Updater.Prereleases()
		}
	}()
	Updater.SetPrereleases(true)
	<-done
	if !Updater.Prereleases() || Updater.Channel() != update.BetaChannel {
		t.Error("SetPrereleases(true) did not switch to beta")
	}
	Updater.SetChannel("nightly")
	if Updater.Channel() != "nightly" {
		t.Error("SetChannel did not apply")
	}
	Updater.SetChannel("")
	if Updater.Prereleases() {
		t.Error("SetChannel(\"\") did not reset")
	}
}

type testSource struct {
	srv      *httptest.Server
	raw      []byte
	queries  []ManifestQuery
	opened   []string
	failNext bool
}

func (s *testSource) Manifest(ctx context.Context, q ManifestQuery) (ManifestResult, error) {
	s.queries = append(s.queries, q)
	return ManifestResult{Body: io.NopCloser(bytes.NewReader(s.raw))}, nil
}

func (s *testSource) Open(ctx context.Context, u string, offset int64) (io.ReadCloser, bool, error) {
	s.opened = append(s.opened, u)
	name, ok := strings.CutPrefix(u, "service:")
	if !ok {
		return nil, false, fmt.Errorf("%s is not a file of the service", u)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.srv.URL+"/"+name, nil)
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	resp, err := s.srv.Client().Do(req)
	if err != nil {
		return nil, false, err
	}
	return resp.Body, resp.StatusCode == http.StatusPartialContent, nil
}

func TestUpdaterSource(t *testing.T) {
	useIsolatedCache(t)
	ctx := context.Background()
	srv, key, _, _ := updateServer(t, "1.2.0", "v2", "")
	resp, err := srv.Client().Get(srv.URL + "/update.json")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	ring, _ := update.ParseKeyring(key)
	m, err := update.VerifyManifest(ring, raw)
	if err != nil {
		t.Fatal(err)
	}
	// Replace URL inside re-signed manifest to use custom service scheme.
	pub, priv, _ := ed25519.GenerateKey(nil)
	m.Artifact.URL = "service:app.tar.zst"
	m.Artifact.Signature = ""
	signed, err := update.SignManifest([]ed25519.PrivateKey{priv}, m)
	if err != nil {
		t.Fatal(err)
	}
	src := &testSource{srv: srv, raw: signed}
	key2 := base64.StdEncoding.EncodeToString(pub)
	up, err := checkUpdate(ctx, src, key2, testTarget(), "beta", "1.1.9", "en")
	if err != nil || up == nil || up.Version != "1.2.0" {
		t.Fatalf("checkUpdate = %+v, %v", up, err)
	}
	if len(src.queries) != 1 || src.queries[0].Channel != "beta" {
		t.Errorf("queries = %+v", src.queries)
	}
	target, main := installedApp(t)
	quit := runningApp(t)
	if err := installUpdate(ctx, up.source.Open, up.manifest, up.keyring, up.current, target, nil); err != nil {
		t.Fatal(err)
	}
	quit()
	if b, _ := os.ReadFile(main); string(b) != "v2" {
		t.Errorf("after update main = %q", b)
	}
	if len(src.opened) != 1 || src.opened[0] != "service:app.tar.zst" {
		t.Errorf("opened = %q", src.opened)
	}

	t.Cleanup(func() { Updater.SetSource(nil) })
	Updater.SetSource(src)
	if s := updateSource.Load(); s == nil || *s != UpdateSource(src) {
		t.Error("SetSource failed")
	}
	Updater.SetSource(nil)
	if updateSource.Load() != nil {
		t.Error("SetSource(nil) failed")
	}
}

func TestHTTPGetRetries(t *testing.T) {
	defer func(r []time.Duration) { httpRetries = r }(httpRetries)
	httpRetries = []time.Duration{time.Millisecond, time.Millisecond}
	var tries atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/missing":
			tries.Add(1)
			http.NotFound(w, r)
		case r.URL.Path == "/down":
			tries.Add(1)
			w.WriteHeader(http.StatusServiceUnavailable)
		case tries.Add(1) < 3:
			w.WriteHeader(http.StatusGatewayTimeout)
		default:
			io.WriteString(w, "ok")
		}
	}))
	defer srv.Close()
	body, err := httpGet(t.Context(), srv.URL+"/file")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(body)
	body.Close()
	if string(b) != "ok" || tries.Load() != 3 {
		t.Errorf("read %q in %d tries", b, tries.Load())
	}
	tries.Store(0)
	if _, err := httpGet(t.Context(), srv.URL+"/down"); err == nil || !strings.Contains(err.Error(), "503") || tries.Load() != 3 {
		t.Errorf("down: %v in %d tries", err, tries.Load())
	}
	tries.Store(0)
	if _, err := httpGet(t.Context(), srv.URL+"/missing"); err == nil || tries.Load() != 1 {
		t.Errorf("missing: %v in %d tries", err, tries.Load())
	}
	if got := retryAfter(" 5 "); got != 5*time.Second {
		t.Errorf("Retry-After 5: %v", got)
	}
}

func TestHTTPGetHeaderTimeout(t *testing.T) {
	transport := httpClient().Transport.(*http.Transport)
	defer func(r []time.Duration, d time.Duration) {
		httpRetries, transport.ResponseHeaderTimeout = r, d
	}(httpRetries, transport.ResponseHeaderTimeout)
	httpRetries = []time.Duration{time.Millisecond}
	transport.ResponseHeaderTimeout = 50 * time.Millisecond

	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		<-block
	}))
	defer srv.Close()
	defer close(block)

	start := time.Now()
	if _, err := httpGet(t.Context(), srv.URL+"/file"); err == nil {
		t.Fatal("a server that never answers returned no error")
	}
	if waited := time.Since(start); waited > 5*time.Second {
		t.Fatalf("waited %v for headers", waited)
	}
}

func TestHTTPGetStallGivesUp(t *testing.T) {
	defer func(d time.Duration) { httpStallTimeout = d }(httpStallTimeout)
	httpStallTimeout = 100 * time.Millisecond

	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		io.WriteString(w, "start")
		w.(http.Flusher).Flush()
		<-block
	}))
	defer srv.Close()
	defer close(block)

	body, err := httpGet(t.Context(), srv.URL+"/file")
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	if _, err := io.ReadAll(body); err == nil || !strings.Contains(err.Error(), "stalled") {
		t.Fatalf("reading a body that stopped ended with %v", err)
	}
}

func TestStalledBodyShortTimeout(t *testing.T) {
	defer func(d time.Duration) { httpStallTimeout = d }(httpStallTimeout)
	httpStallTimeout = time.Nanosecond
	for range 100 {
		ctx, cancel := context.WithCancel(t.Context())
		body := newStalledBody(io.NopCloser(strings.NewReader("")), cancel)
		wait.For(t, "the stalled body to cancel its request", ctx.Done())
		body.Close()
		if !body.stalled.Load() {
			t.Fatal("the body did not report a stall")
		}
	}
}

func TestHTTPGetKeepsASlowBody(t *testing.T) {
	defer func(d time.Duration) { httpStallTimeout = d }(httpStallTimeout)
	httpStallTimeout = 400 * time.Millisecond

	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for i := 0; i < 10; i++ {
			io.WriteString(w, "x")
			w.(http.Flusher).Flush()
			time.Sleep(50 * time.Millisecond)
		}
		<-block
	}))
	defer srv.Close()
	defer close(block)

	body, err := httpGet(t.Context(), srv.URL+"/file")
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	b, err := io.ReadAll(io.LimitReader(body, 10))
	if err != nil || len(b) != 10 {
		t.Fatalf("read %d bytes of a slow body: %v", len(b), err)
	}
}

func TestWorkPattern(t *testing.T) {
	roots := []string{string(filepath.Separator), "."}
	if runtime.GOOS == "windows" {
		roots = append(roots, `D:\`, `\\server\share\`)
	}
	for _, target := range roots {
		pattern := workPattern(target)
		if strings.ContainsFunc(pattern, func(r rune) bool { return r < 0x80 && os.IsPathSeparator(uint8(r)) }) {
			t.Errorf("workPattern(%q) = %q, which holds a separator", target, pattern)
		}
	}
	if got, want := workPattern(filepath.Join(t.TempDir(), "MyApp")), ".MyApp.update-"; got != want {
		t.Errorf("workPattern = %q, want %q", got, want)
	}
}

func TestRelaunchArguments(t *testing.T) {
	args := []string{"--reason", "it's an update", "a b", `$HOME "quoted" \ back`, "line\nbreak", ""}
	onMain(func() {
		if got := relaunchArguments(); !slices.Equal(got, os.Args[1:]) {
			t.Errorf("without arguments: %q, want %q", got, os.Args[1:])
		}
		App.relaunchArgs = args
		defer func() { App.relaunchArgs = nil }()
		if got := relaunchArguments(); !slices.Equal(got, args) {
			t.Errorf("with arguments: %q", got)
		}
	})
	if runtime.GOOS == "windows" {
		return
	}
	dir := t.TempDir()
	target, bundle, work := filepath.Join(dir, "A.app"), filepath.Join(dir, "new", "A.app"), filepath.Join(dir, "work")
	for _, d := range []string{target, bundle, work} {
		_ = os.MkdirAll(d, 0o755)
	}
	_ = os.WriteFile(filepath.Join(work, "relaunch"), []byte(setArgs(openArgs(target, args))), 0o644)
	exited := exec.Command("/bin/sh", "-c", "exit 0")
	_ = exited.Run()
	script := strings.Replace(installAfterExit, "exec /usr/bin/open", `printf '%s\0'`, 1)
	out, err := exec.Command("/bin/sh", "-c", script, "mygo-update", strconv.Itoa(exited.Process.Pid), target, bundle, work).Output()
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(string(out), "\x00")
	if want := openArgs(target, args); !slices.Equal(got[:len(got)-1], want) {
		t.Errorf("relaunch args = %q, want %q", got, want)
	}
	if _, err := os.Stat(work); !os.IsNotExist(err) {
		t.Error("work dir left behind")
	}
}

func TestUpdateCheckRedirect(t *testing.T) {
	req := func(u string) *http.Request {
		r, _ := http.NewRequest("GET", u, nil)
		return r
	}
	via := []*http.Request{req("https://example.com/feed.json")}
	if err := checkRedirect(req("https://cdn.example.com/feed.json"), via); err != nil {
		t.Errorf("to HTTPS: %v", err)
	}
	if err := checkRedirect(req("http://cdn.example.com/feed.json"), via); err == nil {
		t.Error("to plain HTTP: followed")
	}
	if err := checkRedirect(req("http://127.0.0.1:8080/feed.json"), via); err != nil {
		t.Errorf("over loopback: %v", err)
	}
	var asked atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		http.Redirect(w, r, "http://updates.example.invalid/feed.json", http.StatusFound)
	}))
	defer srv.Close()
	if _, err := httpGet(context.Background(), srv.URL+"/feed.json"); !errors.Is(err, errRedirect) {
		t.Errorf("httpGet = %v", err)
	}
	if asked.Load() != 1 {
		t.Errorf("asked %d times", asked.Load())
	}
}
