//go:build !js

package mygo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/internal/update"
	"github.com/egoist/mygo/netproxy"
)

// Set by `mygo build` with -ldflags -X when mygo.json configures updates.
var (
	packageUpdateFeed    string
	packageUpdateKeys    string
	packageUpdateTarget  string
	packageUpdateChannel string
)

// startExe is the executable the app started from. On Linux, os.Executable
// reports the old file once an update replaced it.
var startExe, _ = os.Executable()

// UpdateAvailability describes whether the running build can self-update.
type UpdateAvailability uint8

const (
	UpdateAvailable UpdateAvailability = iota + 1
	UpdateUnavailableUnconfigured
	UpdateUnavailableDev
	UpdateUnavailableTranslocated
	UpdateUnavailableNotBundle
	UpdateUnavailableAppImage
	UpdateUnavailableReadOnly
)

// UpdaterModule installs new versions of the app, built with `mygo build`.
type UpdaterModule struct{}

// Updater installs new versions of the app.
var Updater = &UpdaterModule{}

var (
	updateChannel atomic.Pointer[string]
	updateSource  atomic.Pointer[UpdateSource]
)

// ManifestQuery describes the manifest requested by Check / CheckWith.
type ManifestQuery struct {
	Target  string
	Channel string
	Current string
	Locale  string
	ETag    string
}

// ManifestResult is returned by UpdateSource.Manifest.
type ManifestResult struct {
	Body        io.ReadCloser
	ETag        string
	NotModified bool
}

// CheckOptions configures an update check via CheckWith.
type CheckOptions struct {
	Channel string
	Locale  string
}

// UpdateSource supplies update manifests and asset streams to Updater.
// Every manifest must still be signed by a trusted key in updates.publicKeys /
// updates.publicKey, and every downloaded asset must match the signed SHA-256
// and size in the manifest.
type UpdateSource interface {
	Manifest(ctx context.Context, q ManifestQuery) (ManifestResult, error)
	// Open opens url starting at offset bytes (when offset > 0). partial must
	// be true only when the stream begins at offset (e.g. HTTP 206); false
	// means the stream starts at byte 0 and any partial file must be truncated.
	Open(ctx context.Context, url string, offset int64) (body io.ReadCloser, partial bool, err error)
}

// SetSource replaces the default update feed with s, or restores it when nil.
func (u *UpdaterModule) SetSource(s UpdateSource) {
	if s == nil {
		updateSource.Store(nil)
		return
	}
	updateSource.Store(&s)
}

// Target returns the build's update target (e.g. "darwin-arm64" or "darwin-universal").
func (u *UpdaterModule) Target() string {
	if packageUpdateTarget != "" {
		return packageUpdateTarget
	}
	return runtime.GOOS + "-" + runtime.GOARCH
}

// DefaultChannel returns the release channel the binary was built for.
func (u *UpdaterModule) DefaultChannel() string {
	if packageUpdateChannel != "" {
		return packageUpdateChannel
	}
	return update.DefaultChannel
}

// Channel returns the active update channel.
func (u *UpdaterModule) Channel() string {
	if ch := updateChannel.Load(); ch != nil && *ch != "" {
		return *ch
	}
	return u.DefaultChannel()
}

// SetChannel sets the update channel (e.g. "release", "prerelease"); empty restores DefaultChannel().
func (u *UpdaterModule) SetChannel(channel string) {
	if channel == "" {
		updateChannel.Store(nil)
		return
	}
	ch := channel
	updateChannel.Store(&ch)
}

// SetPrereleases selects the "prerelease" channel when on is true, or "release" when false.
func (u *UpdaterModule) SetPrereleases(on bool) {
	if on {
		u.SetChannel(update.PrereleaseChannel)
		return
	}
	u.SetChannel(update.DefaultChannel)
}

// Prereleases reports whether the active channel is "prerelease".
func (u *UpdaterModule) Prereleases() bool {
	return u.Channel() == update.PrereleaseChannel
}

var (
	// ErrUpdatesDisabled is returned when self-updates are unavailable for this build.
	ErrUpdatesDisabled = errors.New("mygo: updates are not enabled for this build")
	// ErrManualUpdateRequired is returned by Install when the update declares a
	// minimum version newer than the running build.
	ErrManualUpdateRequired = errors.New("mygo: this update requires a manual reinstall")
)

// Update is a verified newer version returned by Updater.Check / CheckWith.
type Update struct {
	Version              string
	Channel              string
	Notes                string
	Date                 time.Time
	RequiresManualUpdate bool
	Sources              []string

	manifest update.Manifest
	keyring  update.Keyring
	source   UpdateSource
	current  string
}

// NotesFor returns release notes best matching locale, falling back to Notes.
func (up *Update) NotesFor(locale string) string {
	if up == nil {
		return ""
	}
	return up.manifest.ResolveNotes(locale)
}

// Availability reports whether this build can self-update and, if not, why.
func (u *UpdaterModule) Availability() UpdateAvailability {
	if IsDev() {
		return UpdateUnavailableDev
	}
	if packageUpdateFeed == "" || packageUpdateKeys == "" {
		return UpdateUnavailableUnconfigured
	}
	if _, err := update.ParseKeyring(packageUpdateKeys); err != nil {
		return UpdateUnavailableUnconfigured
	}
	exe := startExe
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	if runtime.GOOS == "darwin" {
		if strings.Contains(exe, "/AppTranslocation/") {
			return UpdateUnavailableTranslocated
		}
		bundle := bundleFromExe(exe)
		if !strings.HasSuffix(bundle, ".app") {
			return UpdateUnavailableNotBundle
		}
	}
	if runtime.GOOS == "linux" && runningInsideAppImage(exe) {
		return UpdateUnavailableAppImage
	}
	target, err := installTarget()
	if err != nil {
		return UpdateUnavailableNotBundle
	}
	if runtime.GOOS == "darwin" {
		if !probeWritableDir(filepath.Dir(target)) {
			return UpdateUnavailableReadOnly
		}
	} else if !probeWritableDir(filepath.Dir(target)) || !probeWritableDir(target) {
		return UpdateUnavailableReadOnly
	}
	return UpdateAvailable
}

func runningInsideAppImage(exe string) bool {
	if os.Getenv("APPIMAGE") == "" {
		return false
	}
	appDir := os.Getenv("APPDIR")
	if appDir == "" {
		return true
	}
	if real, err := filepath.EvalSymlinks(appDir); err == nil {
		appDir = real
	}
	appDir = filepath.Clean(appDir)
	exe = filepath.Clean(exe)
	return exe == appDir || strings.HasPrefix(exe, appDir+string(filepath.Separator))
}

func probeWritableDir(dir string) bool {
	probe, err := os.MkdirTemp(dir, ".app-update-")
	if err != nil {
		return false
	}
	_ = os.Remove(probe)
	return true
}

// TakeLastInstallError returns and clears any error recorded by the post-exit
// bundle swap helper (macOS) from the previous launch attempt.
func (u *UpdaterModule) TakeLastInstallError() error {
	p := lastInstallErrorPath()
	if p == "" {
		return nil
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	_ = os.Remove(p)
	msg := strings.TrimSpace(string(data))
	if msg == "" {
		return nil
	}
	return errors.New(msg)
}

func lastInstallErrorPath() string {
	cacheBase, err := App.Path(PathCache)
	if err != nil || cacheBase == "" {
		return ""
	}
	return filepath.Join(cacheBase, "updates", "last-install-error.txt")
}

// Enabled reports whether the app can update itself.
func (u *UpdaterModule) Enabled() bool {
	return u.Availability() == UpdateAvailable
}

type etagEntry struct {
	etag string
	raw  []byte
}

var etagCache struct {
	mu      sync.Mutex
	entries map[string]etagEntry
}

func getETagEntry(key string) (etagEntry, bool) {
	etagCache.mu.Lock()
	defer etagCache.mu.Unlock()
	e, ok := etagCache.entries[key]
	return e, ok
}

func setETagEntry(key string, entry etagEntry) {
	etagCache.mu.Lock()
	defer etagCache.mu.Unlock()
	if etagCache.entries == nil {
		etagCache.entries = map[string]etagEntry{}
	}
	etagCache.entries[key] = entry
}

// Check checks the active feed/channel for a newer update.
func (u *UpdaterModule) Check(ctx context.Context) (*Update, error) {
	return u.CheckWith(ctx, CheckOptions{})
}

// CheckWith checks for a newer update with explicit channel and locale preferences.
func (u *UpdaterModule) CheckWith(ctx context.Context, opts CheckOptions) (*Update, error) {
	if !u.Enabled() {
		return nil, ErrUpdatesDisabled
	}
	var src UpdateSource = feedSource(packageUpdateFeed)
	srcKey := "feed:" + packageUpdateFeed
	if s := updateSource.Load(); s != nil {
		src = *s
		srcKey = fmt.Sprintf("custom:%T", src)
	}
	ch := opts.Channel
	if ch == "" {
		ch = u.Channel()
	}
	return checkUpdateCached(ctx, src, srcKey, packageUpdateKeys, u.Target(), ch, App.Version(), opts.Locale)
}

type feedSource string

func (f feedSource) Manifest(ctx context.Context, q ManifestQuery) (ManifestResult, error) {
	feedURL, err := update.ResolveChannelFeed(ctx, string(f), q.Target, q.Channel, httpGetPlain)
	if err != nil {
		return ManifestResult{}, err
	}
	u, err := parseHTTPSURL(feedURL)
	if err != nil {
		return ManifestResult{}, err
	}
	reqHeaders := http.Header{
		"Accept":        {"application/json"},
		"Cache-Control": {"no-cache"},
	}
	if q.ETag != "" {
		reqHeaders.Set("If-None-Match", q.ETag)
	}
	if q.Current != "" {
		reqHeaders.Set("X-Update-Current", q.Current)
	}
	if q.Target != "" {
		reqHeaders.Set("X-Update-Target", q.Target)
	}
	if q.Channel != "" {
		reqHeaders.Set("X-Update-Channel", q.Channel)
	}
	if q.Locale != "" {
		reqHeaders.Set("X-Update-Locale", q.Locale)
	}
	resp, err := httpDoWithRetries(ctx, u.String(), reqHeaders)
	if err != nil {
		return ManifestResult{}, err
	}
	if resp.StatusCode == http.StatusNotModified {
		resp.Body.Close()
		return ManifestResult{NotModified: true, ETag: resp.Header.Get("ETag")}, nil
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return ManifestResult{}, fmt.Errorf("GET %s: %s", u.Redacted(), resp.Status)
	}
	return ManifestResult{Body: resp.Body, ETag: resp.Header.Get("ETag")}, nil
}

func (f feedSource) Open(ctx context.Context, rawURL string, offset int64) (io.ReadCloser, bool, error) {
	return httpGetRange(ctx, rawURL, offset)
}

var checkTimeout = 30 * time.Second

func withDefaultDeadline(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, d)
}

func checkUpdateCached(ctx context.Context, src UpdateSource, srcKey, keysRaw, target, channel, current, locale string) (*Update, error) {
	ctx, cancel := withDefaultDeadline(ctx, checkTimeout)
	defer cancel()
	cacheKey := srcKey + "|" + target + "|" + channel
	cached, hasCached := getETagEntry(cacheKey)
	q := ManifestQuery{Target: target, Channel: channel, Current: current, Locale: locale}
	if hasCached {
		q.ETag = cached.etag
	}
	res, err := src.Manifest(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("mygo: checking for updates: %w", err)
	}
	var raw []byte
	switch {
	case res.NotModified && hasCached:
		raw = cached.raw
	case res.NotModified:
		return nil, errors.New("mygo: checking for updates: server returned 304 without cached manifest")
	default:
		if res.Body == nil {
			return nil, errors.New("mygo: checking for updates: empty manifest body")
		}
		raw, err = io.ReadAll(io.LimitReader(res.Body, update.MaxManifestSize+1))
		res.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("mygo: checking for updates: %w", err)
		}
		if len(raw) > update.MaxManifestSize {
			return nil, errors.New("mygo: checking for updates: manifest too large")
		}
	}
	up, err := verifyAndBuildUpdate(raw, keysRaw, target, current, locale, src)
	if err != nil {
		return nil, fmt.Errorf("mygo: checking for updates: %w", err)
	}
	if res.ETag != "" && !res.NotModified {
		setETagEntry(cacheKey, etagEntry{etag: res.ETag, raw: raw})
	}
	return up, nil
}

func checkUpdate(ctx context.Context, src UpdateSource, keysRaw, target, channel, current, locale string) (*Update, error) {
	res, err := src.Manifest(ctx, ManifestQuery{Target: target, Channel: channel, Current: current, Locale: locale})
	if err != nil {
		return nil, fmt.Errorf("mygo: checking for updates: %w", err)
	}
	if res.Body == nil {
		return nil, errors.New("mygo: checking for updates: empty manifest body")
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, update.MaxManifestSize+1))
	res.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("mygo: checking for updates: %w", err)
	}
	if len(raw) > update.MaxManifestSize {
		return nil, errors.New("mygo: checking for updates: manifest too large")
	}
	up, err := verifyAndBuildUpdate(raw, keysRaw, target, current, locale, src)
	if err != nil {
		return nil, fmt.Errorf("mygo: checking for updates: %w", err)
	}
	return up, nil
}

func verifyAndBuildUpdate(raw []byte, keysRaw, target, current, locale string, src UpdateSource) (*Update, error) {
	ring, err := update.ParseKeyring(keysRaw)
	if err != nil {
		return nil, err
	}
	m, err := update.VerifyManifest(ring, raw)
	if err != nil {
		return nil, err
	}
	if target != "" && m.Target != target {
		return nil, fmt.Errorf("manifest target %q does not match %q", m.Target, target)
	}
	if !update.SystemSatisfied(m.MinSystem, currentOSVersion()) {
		return nil, nil
	}
	if update.Compare(m.Version, current) <= 0 {
		return nil, nil
	}
	up := &Update{
		Version:              m.Version,
		Channel:              m.Channel,
		Notes:                m.ResolveNotes(locale),
		RequiresManualUpdate: m.MinVersion != "" && update.Compare(current, m.MinVersion) < 0,
		Sources:              m.Artifact.CandidateURLs(m.Mirrors, m.Version),
		manifest:             *m,
		keyring:              ring,
		source:               src,
		current:              current,
	}
	up.Date, _ = time.Parse(time.RFC3339, m.Date)
	return up, nil
}

// Install downloads the delta or archive for up across its candidate mirrors
// with Range resume, verifies its SHA-256 and size against the signed manifest,
// and installs/stages it in place.
func (up *Update) Install(ctx context.Context, progress func(downloaded, total int64)) error {
	if up == nil {
		return errors.New("mygo: nil update")
	}
	if up.RequiresManualUpdate {
		return ErrManualUpdateRequired
	}
	target, err := installTarget()
	if err != nil {
		return err
	}
	open := httpGetRange
	if up.source != nil {
		open = up.source.Open
	}
	if err := installUpdate(ctx, open, up.manifest, up.keyring, up.current, target, progress); err != nil {
		return fmt.Errorf("mygo: installing the update: %w", err)
	}
	switch runtime.GOOS {
	case "linux":
		if err := update.RefreshDesktopEntry(target, filepath.Base(startExe)); err != nil {
			log.Printf("mygo: registering the desktop entry of the update: %v", err)
		}
	case "windows":
		info, _ := packageInfo()
		if err := update.RefreshUninstallEntry(target, info.Identifier, up.Version); err != nil {
			log.Printf("mygo: registering the version of the update in Settings > Apps: %v", err)
		}
	}
	return nil
}

func bundleFromExe(exe string) string {
	return filepath.Dir(filepath.Dir(filepath.Dir(exe)))
}

func installTarget() (string, error) {
	exe := startExe
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	if runtime.GOOS == "darwin" {
		bundle := bundleFromExe(exe)
		if !strings.HasSuffix(bundle, ".app") {
			return "", errors.New("mygo: only app bundles can update themselves")
		}
		return bundle, nil
	}
	if runningInsideAppImage(exe) {
		return "", errors.New("mygo: AppImages cannot update themselves")
	}
	return filepath.Dir(exe), nil
}

type mirrorOpener func(ctx context.Context, url string, offset int64) (io.ReadCloser, bool, error)

func installUpdate(ctx context.Context, open mirrorOpener, m update.Manifest, ring update.Keyring, current, target string, progress func(downloaded, total int64)) error {
	if len(ring) == 0 {
		return errors.New("no trusted update keys")
	}
	work, err := os.MkdirTemp(filepath.Dir(target), workPattern(target))
	if err != nil {
		return fmt.Errorf("cannot write next to the app: %w", err)
	}
	_ = os.WriteFile(filepath.Join(work, ".active"), []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600)
	staged := false
	defer func() {
		if !staged {
			os.RemoveAll(work)
		}
	}()
	unpacked := filepath.Join(work, "app")
	swap := swapFiles
	if runtime.GOOS == "darwin" {
		swap = func(dir, target string) error {
			err := stageBundle(work, dir, target)
			staged = err == nil
			return err
		}
	}

	if d := m.Delta(current); d != nil {
		err := applyDelta(ctx, open, m, *d, ring, target, work, unpacked, progress)
		if err == nil {
			return swap(unpacked, target)
		}
		if ctx.Err() != nil {
			return err
		}
		log.Printf("mygo: downloading the whole update, as its delta update failed: %v", err)
		if err := os.RemoveAll(unpacked); err != nil {
			return err
		}
	}

	archivePath := filepath.Join(work, "update.archive")
	urls := m.Artifact.CandidateURLs(m.Mirrors, m.Version)
	if err := downloadMirrors(ctx, open, urls, m.Artifact.Size, m.Artifact.SHA256, m.Artifact.Signature, ring, m.Target, archivePath, progress); err != nil {
		return err
	}
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	err = update.ExtractArchive(f, unpacked)
	f.Close()
	if err != nil {
		return err
	}
	return swap(unpacked, target)
}

func workPattern(target string) string {
	name := filepath.Base(target)
	if name == "." || strings.ContainsFunc(name, func(r rune) bool { return r < 0x80 && os.IsPathSeparator(uint8(r)) }) {
		name = "app"
	}
	return "." + name + ".update-"
}

func applyDelta(ctx context.Context, open mirrorOpener, m update.Manifest, d update.Delta, ring update.Keyring, target, work, dir string, progress func(downloaded, total int64)) error {
	path := filepath.Join(work, "update.delta")
	urls := d.CandidateURLs(m.Mirrors, m.Version)
	if err := downloadMirrors(ctx, open, urls, d.Size, d.SHA256, d.Signature, ring, m.Target, path, progress); err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if runtime.GOOS == "darwin" {
		if err := os.Mkdir(dir, 0o755); err != nil {
			return err
		}
		dir = filepath.Join(dir, filepath.Base(target))
	}
	return update.ApplyDelta(f, d.Size, d.From, m.Version, target, dir)
}

func updateCacheDir() string {
	if base, err := App.Path(PathCache); err == nil && base != "" {
		dir := filepath.Join(base, "updates")
		if os.MkdirAll(dir, 0o755) == nil {
			return dir
		}
	}
	dir := filepath.Join(os.TempDir(), "app-updates")
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

// downloadMirrors downloads from urls into a resumable .part cache file under
// PathCache/updates, rotating across mirrors and resuming via HTTP Range on
// transient stream failures, then verifies exact size and SHA-256 (and optional
// standalone artifact signature) before placing the verified file at dstPath.
func downloadMirrors(
	ctx context.Context,
	open mirrorOpener,
	urls []string,
	size int64,
	wantSHA256, wantSig string,
	ring update.Keyring,
	target, dstPath string,
	progress func(downloaded, total int64),
) error {
	if len(urls) == 0 {
		return errors.New("no download URLs")
	}
	if size <= 0 || size > update.MaxArchiveSize {
		return fmt.Errorf("invalid size %d", size)
	}
	if !update.ValidSHA256(wantSHA256) {
		return errors.New("invalid expected sha256")
	}
	if target == "" {
		target = "app"
	}
	partPath := filepath.Join(updateCacheDir(), fmt.Sprintf("%s-%s.part", target, wantSHA256[:min(16, len(wantSHA256))]))

	verifyPart := func() error {
		sum, err := verifyFileDigest(partPath, size, wantSHA256, wantSig, ring)
		if err != nil {
			return err
		}
		_ = sum
		if err := moveVerifiedFile(partPath, dstPath); err != nil {
			return err
		}
		if progress != nil {
			progress(size, size)
		}
		return nil
	}

	// Already completely downloaded from an earlier run?
	if info, err := os.Stat(partPath); err == nil && info.Size() == size {
		if err := verifyPart(); err == nil {
			return nil
		}
		_ = os.Remove(partPath)
	} else if err == nil && info.Size() > size {
		_ = os.Remove(partPath)
	}

	maxAttempts := max(len(urls)*2+1, 3)
	var lastErr error
	triedClean := make([]bool, len(urls))
	mirrorIdx := 0
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			wait := time.Duration(min(attempt, 3)) * 500 * time.Millisecond
			if mirrorRetryWait != nil {
				if !mirrorRetryWait(ctx, wait) {
					return ctx.Err()
				}
			}
		}
		u := urls[mirrorIdx%len(urls)]
		offset := int64(0)
		if info, err := os.Stat(partPath); err == nil && info.Size() > 0 && info.Size() < size {
			offset = info.Size()
		}

		attemptCtx, cancelAttempt := context.WithTimeout(ctx, httpAttemptTimeout)
		body, partial, err := open(attemptCtx, u, offset)
		if err != nil {
			cancelAttempt()
			lastErr = err
			if ctx.Err() != nil || errors.Is(err, errRedirect) {
				return err
			}
			if errors.Is(err, errRangeNotSatisfiable) {
				_ = os.Remove(partPath)
				// Retry same or next mirror from offset 0 cleanly.
				if offset > 0 {
					continue
				}
			}
			if offset == 0 {
				triedClean[mirrorIdx%len(urls)] = true
			}
			mirrorIdx = nextUntriedMirror(triedClean, mirrorIdx)
			continue
		}

		flag := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
		startOffset := int64(0)
		if offset > 0 && partial {
			flag = os.O_WRONLY | os.O_APPEND
			startOffset = offset
		}
		f, err := os.OpenFile(partPath, flag, 0o644)
		if err != nil {
			body.Close()
			cancelAttempt()
			return err
		}
		pr := &progressReader{r: io.LimitReader(body, size-startOffset+1), base: startOffset, total: size, fn: progress}
		n, copyErr := io.Copy(f, pr)
		closeErr := f.Close()
		body.Close()
		cancelAttempt()
		if copyErr == nil {
			copyErr = closeErr
		}
		if copyErr != nil {
			lastErr = fmt.Errorf("downloading: %w", copyErr)
			if ctx.Err() != nil {
				return lastErr
			}
			// If a fresh HTTP 200 connection timed out/stalled before sending
			// meaningful data, discard tiny prefix bytes so fallback mirrors
			// start cleanly from offset 0 without corrupting Range resume.
			if !partial && startOffset == 0 && attemptCtx.Err() != nil && n < 4096 {
				_ = os.Remove(partPath)
				triedClean[mirrorIdx%len(urls)] = true
			}
			mirrorIdx = nextUntriedMirror(triedClean, mirrorIdx)
			continue
		}
		totalGot := startOffset + n
		if totalGot != size {
			// If the server gave a full 200 short body (not partial), it sent
			// an invalid/truncated response from offset 0 — discard it so the
			// next mirror starts cleanly from byte 0 rather than appending tail
			// onto corrupt bytes.
			if totalGot > size || !partial {
				_ = os.Remove(partPath)
				if !partial && startOffset == 0 {
					triedClean[mirrorIdx%len(urls)] = true
				}
			}
			lastErr = fmt.Errorf("downloaded %d bytes, want %d", totalGot, size)
			mirrorIdx = nextUntriedMirror(triedClean, mirrorIdx)
			continue
		}
		if err := verifyPart(); err != nil {
			_ = os.Remove(partPath)
			lastErr = err
			if startOffset == 0 {
				triedClean[mirrorIdx%len(urls)] = true
			}
			next := nextUntriedMirror(triedClean, mirrorIdx)
			if attempt+1 < maxAttempts && !triedClean[next] {
				mirrorIdx = next
				continue
			}
			if startOffset > 0 && !triedClean[mirrorIdx%len(urls)] && attempt+1 < maxAttempts {
				continue
			}
			return err
		}
		return nil
	}
	if lastErr == nil {
		lastErr = errors.New("download failed")
	}
	return lastErr
}

func nextUntriedMirror(tried []bool, cur int) int {
	for step := 1; step <= len(tried); step++ {
		idx := (cur + step) % len(tried)
		if !tried[idx] {
			return idx
		}
	}
	return (cur + 1) % len(tried)
}

var mirrorRetryWait = func(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func verifyFileDigest(path string, wantSize int64, wantSHA256, wantSig string, ring update.Keyring) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sum := sha256.New()
	n, err := io.Copy(sum, f)
	if err != nil {
		return nil, err
	}
	if n != wantSize {
		return nil, fmt.Errorf("downloaded %d bytes, want %d", n, wantSize)
	}
	digest := sum.Sum(nil)
	if !strings.EqualFold(hex.EncodeToString(digest), wantSHA256) {
		return nil, errors.New("update checksum mismatch")
	}
	if wantSig != "" {
		if err := update.VerifyArtifactAny(ring, digest, wantSig); err != nil {
			return nil, err
		}
	}
	return digest, nil
}

func moveVerifiedFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		in.Close()
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeIn := in.Close()
	closeOut := out.Close()
	if copyErr != nil {
		_ = os.Remove(dst)
		return copyErr
	}
	if closeOut != nil {
		_ = os.Remove(dst)
		return closeOut
	}
	_ = closeIn
	_ = os.Remove(src)
	return nil
}

type stagedUpdate struct {
	work, bundle, target string
	helper               *exec.Cmd
}

var pendingUpdate atomic.Pointer[stagedUpdate]

var appPid = os.Getpid()

func stageBundle(work, dir, target string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(entries) != 1 || !strings.HasSuffix(entries[0].Name(), ".app") {
		return errors.New("the update holds no app bundle")
	}
	errFile := lastInstallErrorPath()
	if errFile != "" {
		_ = os.MkdirAll(filepath.Dir(errFile), 0o755)
		_ = os.Remove(errFile)
	}
	p := &stagedUpdate{work: work, bundle: filepath.Join(dir, entries[0].Name()), target: target}
	p.helper = exec.Command("/bin/sh", "-c", installAfterExit, "app-update", strconv.Itoa(appPid), p.target, p.bundle, p.work, errFile)
	if err := p.helper.Start(); err != nil {
		return err
	}
	if old := pendingUpdate.Swap(p); old != nil {
		_ = old.helper.Process.Kill()
		_ = old.helper.Wait()
		_ = os.RemoveAll(old.work)
	}
	return nil
}

const installAfterExit = `pid=$1 target=$2 new=$3 work=$4 errfile=$5
trap '' HUP
while kill -0 "$pid" 2>/dev/null; do sleep 0.05; done
record_err() {
	if [ -n "$errfile" ]; then
		mkdir -p "$(dirname "$errfile")" 2>/dev/null || true
		printf '%s\n' "$1" > "$errfile" 2>/dev/null || true
	fi
}
if err=$(mv "$target" "$work/old" 2>&1); then
	if err2=$(mv "$new" "$target" 2>&1); then
		:
	else
		mv "$work/old" "$target" 2>/dev/null || true
		record_err "could not move new app into place: $err2"
	fi
else
	record_err "could not replace installed app: $err"
fi
relaunch=
if [ -e "$work/relaunch" ]; then relaunch=1; . "$work/relaunch"; fi
rm -rf "$work"
if [ -n "$relaunch" ]; then exec /usr/bin/open "$@"; fi
`

func relaunchUpdated(args []string) bool {
	p := pendingUpdate.Load()
	return p != nil && os.WriteFile(filepath.Join(p.work, "relaunch"), []byte(setArgs(openArgs(p.target, args))), 0o644) == nil
}

func openArgs(bundle string, args []string) []string {
	if len(args) == 0 {
		return []string{bundle}
	}
	return append([]string{bundle, "--args"}, args...)
}

func setArgs(args []string) string {
	var b strings.Builder
	b.WriteString("set --")
	for _, a := range args {
		b.WriteString(" '")
		b.WriteString(strings.ReplaceAll(a, "'", `'\''`))
		b.WriteString("'")
	}
	b.WriteString("\n")
	return b.String()
}

func swapFiles(dir, target string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var names []string
	ours := map[string]bool{}
	for _, e := range entries {
		if e.Name() != update.FileList {
			names = append(names, e.Name())
			ours[e.Name()] = true
		}
	}
	if len(names) == 0 {
		return errors.New("the update is empty")
	}
	suffix := ".old-" + strconv.Itoa(os.Getpid())
	var moved, placed [][2]string
	undo := func() {
		for i := len(placed) - 1; i >= 0; i-- {
			if err := rename(placed[i][1], placed[i][0]); err != nil {
				if rmErr := os.RemoveAll(placed[i][1]); rmErr != nil {
					log.Printf("mygo: reverting placed file %s: %v (remove: %v)", placed[i][1], err, rmErr)
				}
			}
		}
		for i := len(moved) - 1; i >= 0; i-- {
			if err := rename(moved[i][1], moved[i][0]); err != nil {
				log.Printf("mygo: restoring %s from %s: %v", moved[i][0], moved[i][1], err)
			}
		}
	}
	moveAside := func(dst string) error {
		aside := filepath.Join(target, "."+filepath.Base(dst)+suffix)
		if err := rename(dst, aside); err != nil {
			return err
		}
		moved = append(moved, [2]string{dst, aside})
		return nil
	}

	running := runningEntry(target)
	var left []string
	for _, name := range installedEntries(target) {
		dst := filepath.Join(target, name)
		if ours[name] || name == running {
			continue
		}
		if _, err := os.Lstat(dst); err != nil {
			continue
		}
		if err := moveAside(dst); err != nil {
			log.Printf("mygo: leaving %s, which the update no longer has: %v", dst, err)
			left = append(left, name)
		}
	}
	if err := update.WriteFileList(dir, append(names, left...)); err != nil {
		undo()
		return err
	}

	for _, name := range append(names, update.FileList) {
		src, dst := filepath.Join(dir, name), filepath.Join(target, name)
		if _, err := os.Lstat(dst); err == nil {
			if err := moveAside(dst); err != nil {
				undo()
				return err
			}
		}
		if err := rename(src, dst); err != nil {
			undo()
			return err
		}
		placed = append(placed, [2]string{src, dst})
	}
	for _, m := range moved {
		_ = os.RemoveAll(m[1])
	}
	return nil
}

func installedEntries(target string) []string {
	b, err := readAppFile(filepath.Join(target, update.FileList))
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			log.Printf("mygo: removing none of the files that the update no longer has: %v", err)
		}
		return nil
	}
	return update.ParseFileList(b)
}

func runningEntry(target string) string {
	if startExe == "" {
		return ""
	}
	exe, dir := startExe, target
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	if filepath.Dir(exe) != filepath.Clean(dir) {
		return ""
	}
	return filepath.Base(exe)
}

func relaunchNow() {
	args := relaunchArguments()
	if launchedByDev() {
		devRelaunch(App.relaunchArgs)
	}
	if runtime.GOOS == "darwin" && (relaunchUpdated(args) || openBundle(args)) {
		return
	}
	cmd := exec.Command(startExe, args...)
	cmd.Dir = startDir
	if runtime.GOOS != "windows" {
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	}
	if cmd.Start() == nil {
		_ = cmd.Process.Release()
	}
}

func devRelaunch(args []string) {
	if args != nil {
		if devRelaunchFile == "" {
			log.Print("mygo: this mygo dev cannot pass Relaunch's arguments to the build; update it")
		} else if err := os.WriteFile(devRelaunchFile, devRelaunchData(args), 0o600); err != nil {
			log.Printf("mygo: cannot pass Relaunch's arguments to mygo dev: %v", err)
		}
	}
	os.Exit(devRelaunchCode)
}

func devRelaunchData(args []string) []byte {
	var b []byte
	for _, a := range args {
		b = append(append(b, a...), 0)
	}
	return b
}

func openBundle(args []string) bool {
	bundle, err := installTarget()
	if err != nil {
		return false
	}
	return exec.Command("/usr/bin/open", append([]string{"-n"}, openArgs(bundle, args)...)...).Run() == nil
}

// cleanUpdateLeftovers removes stale .update-* staging folders (on all OSes),
// Windows/Linux .old-* leftovers, and expired .part downloads.
func cleanUpdateLeftovers() {
	now := time.Now()
	if runtime.GOOS != "darwin" {
		dirs := map[string]bool{}
		if startExe != "" {
			dirs[filepath.Dir(startExe)] = true
		}
		if target, err := installTarget(); err == nil {
			dirs[target] = true
		}
		for dir := range dirs {
			olds, _ := filepath.Glob(filepath.Join(dir, ".*.old-*"))
			for _, p := range olds {
				_ = os.RemoveAll(p)
			}
		}
	}
	if target, err := installTarget(); err == nil {
		if stages, err := filepath.Glob(filepath.Join(filepath.Dir(target), workPattern(target)+"*")); err == nil {
			for _, p := range stages {
				if workHasLivePID(p) {
					continue
				}
				if info, err := os.Stat(p); err == nil && now.Sub(info.ModTime()) > 5*time.Minute {
					_ = os.RemoveAll(p)
				}
			}
		}
	}
	if cacheBase, err := App.Path(PathCache); err == nil && cacheBase != "" {
		parts, _ := filepath.Glob(filepath.Join(cacheBase, "updates", "*.part"))
		for _, p := range parts {
			if info, err := os.Stat(p); err == nil && now.Sub(info.ModTime()) > 7*24*time.Hour {
				_ = os.Remove(p)
			}
		}
	}
}

var currentOSVersion = sync.OnceValue(func() string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	b, err := os.ReadFile("/System/Library/CoreServices/SystemVersion.plist")
	if err != nil {
		return ""
	}
	s := string(b)
	const key = "<key>ProductVersion</key>"
	_, rest, ok := strings.Cut(s, key)
	if !ok {
		return ""
	}
	_, rest, ok = strings.Cut(rest, "<string>")
	if !ok {
		return ""
	}
	ver, _, ok := strings.Cut(rest, "</string>")
	if !ok {
		return ""
	}
	return strings.TrimSpace(ver)
})

func parseHTTPSURL(rawURL string) (*url.URL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && isLoopback(u.Hostname())) {
		return nil, fmt.Errorf("refusing to download %s: updates need HTTPS", rawURL)
	}
	return u, nil
}

func httpGetPlain(ctx context.Context, rawURL string) (io.ReadCloser, error) {
	body, _, err := httpGetRange(ctx, rawURL, 0)
	return body, err
}

func httpGet(ctx context.Context, rawURL string) (io.ReadCloser, error) {
	return httpGetPlain(ctx, rawURL)
}

func httpGetRange(ctx context.Context, rawURL string, offset int64) (io.ReadCloser, bool, error) {
	if _, err := parseHTTPSURL(rawURL); err != nil {
		return nil, false, err
	}
	var headers http.Header
	if offset > 0 {
		headers = http.Header{"Range": {"bytes=" + strconv.FormatInt(offset, 10) + "-"}}
	}
	resp, err := httpDoWithRetries(ctx, rawURL, headers)
	if err != nil {
		return nil, false, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return resp.Body, false, nil
	case http.StatusPartialContent:
		if offset > 0 && validContentRangeStart(resp.Header.Get("Content-Range"), offset) {
			return resp.Body, true, nil
		}
		resp.Body.Close()
		return nil, false, fmt.Errorf("GET %s: unexpected Content-Range %q", rawURL, resp.Header.Get("Content-Range"))
	default:
		resp.Body.Close()
		if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
			return nil, false, fmt.Errorf("%w: GET %s: %s", errRangeNotSatisfiable, rawURL, resp.Status)
		}
		return nil, false, fmt.Errorf("GET %s: %s", rawURL, resp.Status)
	}
}

var errRangeNotSatisfiable = errors.New("range not satisfiable")

func validContentRangeStart(header string, offset int64) bool {
	rest, ok := strings.CutPrefix(strings.TrimSpace(header), "bytes ")
	if !ok {
		return false
	}
	startStr, _, ok := strings.Cut(rest, "-")
	if !ok {
		return false
	}
	start, err := strconv.ParseInt(strings.TrimSpace(startStr), 10, 64)
	return err == nil && start == offset
}

func httpDoWithRetries(ctx context.Context, rawURL string, headers http.Header) (*http.Response, error) {
	if _, err := parseHTTPSURL(rawURL); err != nil {
		return nil, err
	}
	client := httpClient()
	for try := 0; ; try++ {
		reqCtx, cancel := context.WithCancel(ctx)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
		if err != nil {
			cancel()
			return nil, err
		}
		req.Header.Set("User-Agent", "app-updater")
		for k, vals := range headers {
			for _, v := range vals {
				req.Header.Add(k, v)
			}
		}
		resp, err := client.Do(req)
		var wait time.Duration
		switch {
		case err != nil:
			cancel()
			if ctx.Err() != nil || try >= len(httpRetries) || errors.Is(err, errRedirect) {
				return nil, err
			}
		case resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusPartialContent:
			resp.Body = newStalledBody(resp.Body, cancel)
			return resp, nil
		case resp.StatusCode == http.StatusNotModified:
			cancel()
			return resp, nil
		default:
			resp.Body.Close()
			cancel()
			if !transientStatus(resp.StatusCode) || try >= len(httpRetries) {
				return resp, fmt.Errorf("GET %s: %s", rawURL, resp.Status)
			}
			wait = retryAfter(resp.Header.Get("Retry-After"))
		}
		wait = max(wait, httpRetries[try])
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
}

var httpHeaderTimeout = 30 * time.Second
var httpStallTimeout = time.Minute
var httpAttemptTimeout = 15 * time.Minute

func workHasLivePID(workDir string) bool {
	data, err := os.ReadFile(filepath.Join(workDir, ".active"))
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return false
	}
	return pidAlive(pid)
}

var updateHTTPClient struct {
	once   sync.Once
	client *http.Client
}

func httpClient() *http.Client {
	updateHTTPClient.once.Do(func() { updateHTTPClient.client = newHTTPClient() })
	return updateHTTPClient.client
}

func newHTTPClient() *http.Client {
	transport := netproxy.Transport().Clone()
	transport.ResponseHeaderTimeout = httpHeaderTimeout
	return &http.Client{Transport: transport, CheckRedirect: checkRedirect}
}

func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return fmt.Errorf("%w: stopped after 10", errRedirect)
	}
	if req.URL.Scheme != "https" && !(req.URL.Scheme == "http" && isLoopback(req.URL.Hostname())) {
		return fmt.Errorf("%w to %s: updates need HTTPS", errRedirect, req.URL.Redacted())
	}
	return nil
}

var errRedirect = errors.New("refusing the redirect")

type stalledBody struct {
	body    io.ReadCloser
	cancel  context.CancelFunc
	d       time.Duration
	last    atomic.Int64
	stalled atomic.Bool
	done    atomic.Bool
	timer   *time.Timer
}

func newStalledBody(body io.ReadCloser, cancel context.CancelFunc) *stalledBody {
	b := &stalledBody{body: body, cancel: cancel, d: httpStallTimeout}
	b.last.Store(time.Now().UnixNano())
	b.timer = time.AfterFunc(1<<63-1, b.check)
	b.timer.Stop()
	b.timer.Reset(b.d / 4)
	return b
}

func (b *stalledBody) check() {
	if b.done.Load() {
		return
	}
	if time.Since(time.Unix(0, b.last.Load())) >= b.d {
		b.stalled.Store(true)
		b.cancel()
		return
	}
	b.timer.Reset(b.d / 4)
}

func (b *stalledBody) Read(p []byte) (int, error) {
	n, err := b.body.Read(p)
	if n > 0 {
		b.last.Store(time.Now().UnixNano())
	}
	if err != nil && b.stalled.Load() {
		return n, errors.New("the download stalled: the connection sent nothing")
	}
	return n, err
}

func (b *stalledBody) Close() error {
	b.done.Store(true)
	b.timer.Stop()
	b.cancel()
	return b.body.Close()
}

var httpRetries = []time.Duration{time.Second, 3 * time.Second}

func transientStatus(code int) bool {
	switch code {
	case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

func retryAfter(h string) time.Duration {
	n, err := strconv.Atoi(strings.TrimSpace(h))
	if err != nil || n <= 0 {
		return 0
	}
	return min(time.Duration(n)*time.Second, 30*time.Second)
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

type progressReader struct {
	r     io.Reader
	n     int64
	base  int64
	total int64
	fn    func(downloaded, total int64)
	last  time.Time
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.n += int64(n)
	cur := p.base + p.n
	if p.fn != nil && (err != nil || cur >= p.total || time.Since(p.last) > 100*time.Millisecond) {
		p.last = time.Now()
		p.fn(min(cur, p.total), p.total)
	}
	return n, err
}
