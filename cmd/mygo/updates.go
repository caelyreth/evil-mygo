package main

import (
	"cmp"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/internal/update"
)

// Updates configures signed updates (v2).
type Updates struct {
	// PublicKey and PublicKeys configure the Ed25519 keyring embedded into builds.
	PublicKey  string   `json:"publicKey"`
	PublicKeys []string `json:"publicKeys"`
	// GitHub is the public repository ("owner/name") hosting updates.
	GitHub    string `json:"github"`
	TagPrefix string `json:"tagPrefix"`
	// FeedURL is the optional dedicated manifest server base URL
	// (e.g. "https://updates.example.com/my-app"). When empty, URL is used.
	FeedURL string `json:"feedUrl"`
	// URL is the primary base URL serving update artifacts (e.g. Cloudflare R2).
	URL string `json:"url"`
	// Mirrors are additional fallback artifact base URLs (supporting {version}).
	Mirrors []string `json:"mirrors"`
	// Channel names the release channel (default: "prerelease" for prereleases, else "release").
	Channel string `json:"channel"`
	// S3 is the S3/R2 bucket where `mygo build -upload` pushes artifacts.
	S3 *S3 `json:"s3"`
	// PrivateKey is the path to mygo-update.key; MYGO_UPDATER_PRIVATE_KEY
	// takes precedence and may hold multiple comma/newline-separated keys.
	PrivateKey string `json:"privateKey"`
	// Changelog is the Markdown file or folder of localized Markdown files for
	// "## <version>" or "### <version>" release notes (default CHANGELOG.md or changelogs/).
	Changelog string `json:"changelog"`
	// Deltas is how many earlier versions receive delta updates (default 3, 0 for none).
	Deltas *int `json:"deltas"`
}

func (u *Updates) deltas() int {
	if u.Deltas == nil {
		return 3
	}
	return *u.Deltas
}

func (u *Updates) tagged() bool {
	return u.GitHub != "" && u.TagPrefix != "v"
}

func (u *Updates) effectiveFeed() string {
	return cmp.Or(u.FeedURL, u.URL)
}

func (u *Updates) effectiveChannel(version string) string {
	if u.Channel != "" {
		return u.Channel
	}
	if update.IsPrerelease(version) {
		return update.PrereleaseChannel
	}
	return update.DefaultChannel
}

func (u *Updates) splitHosting() bool {
	return u.FeedURL != "" && u.URL != "" &&
		strings.TrimSuffix(u.FeedURL, "/") != strings.TrimSuffix(u.URL, "/")
}

func (u *Updates) effectiveKeyring() (update.Keyring, string, error) {
	var rawKeys []string
	if strings.TrimSpace(u.PublicKey) != "" {
		rawKeys = append(rawKeys, u.PublicKey)
	}
	rawKeys = append(rawKeys, u.PublicKeys...)
	if len(rawKeys) == 0 {
		return nil, "", errors.New("updates needs publicKey or publicKeys of mygo keygen")
	}
	ring, err := update.ParseKeyring(strings.Join(rawKeys, ","))
	if err != nil {
		return nil, "", err
	}
	pubs := make([]ed25519.PublicKey, len(ring))
	for i, tk := range ring {
		pubs[i] = tk.Key
	}
	return ring, update.EncodeKeyring(pubs), nil
}

func (u *Updates) validate() error {
	if _, _, err := u.effectiveKeyring(); err != nil {
		return fmt.Errorf("updates keyring: %w", err)
	}
	hasCustom := u.URL != "" || u.FeedURL != ""
	switch {
	case (u.GitHub == "") == !hasCustom:
		return errors.New("updates needs either github (owner/name) or url/feedUrl")
	case u.GitHub != "" && strings.Count(u.GitHub, "/") != 1:
		return fmt.Errorf("updates.github %q is not owner/name", u.GitHub)
	}
	if u.URL != "" {
		if err := validateHTTPSBase("updates.url", u.URL, false, false); err != nil {
			return err
		}
	}
	if u.FeedURL != "" {
		if err := validateHTTPSBase("updates.feedUrl", u.FeedURL, true, true); err != nil {
			return err
		}
	}
	for _, m := range u.Mirrors {
		if err := validateHTTPSBase("updates.mirrors", m, false, true); err != nil {
			return err
		}
	}
	if u.S3 != nil {
		if u.URL == "" {
			return errors.New("updates.s3 needs url, the HTTPS URL that serves the bucket")
		}
		if err := u.S3.validate(); err != nil {
			return err
		}
	}
	if u.Deltas != nil && *u.Deltas < 0 {
		return fmt.Errorf("updates.deltas is %d, not a number of versions", *u.Deltas)
	}
	if u.TagPrefix == "" {
		u.TagPrefix = "v"
	}
	return nil
}

func validateHTTPSBase(field, raw string, allowQueryTemplate, allowPlaceholders bool) error {
	p, err := url.Parse(raw)
	if err != nil || p.Scheme != "https" || p.Host == "" {
		return fmt.Errorf("%s %q is not an https URL", field, raw)
	}
	if !allowQueryTemplate && strings.ContainsAny(raw, "?#") {
		return fmt.Errorf("%s %q has a query or fragment", field, raw)
	}
	if allowQueryTemplate && strings.Contains(raw, "?") && !strings.Contains(raw, "{") {
		return fmt.Errorf("%s %q has a query without template placeholders", field, raw)
	}
	_ = allowPlaceholders
	return nil
}

func (c *Config) updateFeed(target string) string {
	return c.updateFeedFor(target, c.Updates.effectiveChannel(c.Version))
}

func (c *Config) updateFeedFor(target, channel string) string {
	u := c.Updates
	if u.GitHub == "" {
		res, err := update.ResolveChannelFeed(context.Background(), u.effectiveFeed(), target, channel, nil)
		if err == nil {
			return res
		}
		return strings.TrimSuffix(u.effectiveFeed(), "/") + "/" + url.PathEscape(channel) + "/" + update.ManifestName(target)
	}
	name := update.ManifestName(target)
	if u.tagged() {
		return update.TaggedFeed(u.GitHub, u.TagPrefix, name)
	}
	return "https://github.com/" + u.GitHub + "/releases/latest/download/" + url.PathEscape(name)
}

func (c *Config) historyFeed(target, channel string) string {
	u := c.Updates
	if u.GitHub != "" {
		return ""
	}
	base := cmp.Or(u.URL, u.effectiveFeed())
	return strings.TrimSuffix(base, "/") + "/" + url.PathEscape(channel) + "/" + update.HistoryName(target)
}

func (c *Config) updateFile(name string, latest bool) string {
	u := c.Updates
	if u.GitHub == "" {
		return joinArtifactURL(cmp.Or(u.URL, u.effectiveFeed()), c.Version, name)
	}
	if latest && u.tagged() {
		return update.TaggedFeed(u.GitHub, u.TagPrefix, name)
	}
	if latest {
		return "https://github.com/" + u.GitHub + "/releases/latest/download/" + url.PathEscape(name)
	}
	return "https://github.com/" + u.GitHub + "/releases/download/" + url.PathEscape(u.TagPrefix+c.Version) + "/" + url.PathEscape(name)
}

func (c *Config) artifactURLs(name string) (primary string, extra []string) {
	u := c.Updates
	if u.GitHub != "" {
		return c.updateFile(name, false), nil
	}
	primary = joinArtifactURL(u.URL, c.Version, name)
	for _, mirror := range u.Mirrors {
		extra = append(extra, joinArtifactURL(mirror, c.Version, name))
	}
	return primary, extra
}

func joinArtifactURL(base, version, name string) string {
	if strings.Contains(base, update.VersionPlaceholder) {
		base = strings.ReplaceAll(base, update.VersionPlaceholder, url.PathEscape(version))
	}
	return strings.TrimSuffix(base, "/") + "/" + url.PathEscape(name)
}

func updateFlags(c *Config, target string) string {
	if c.Updates == nil {
		return ""
	}
	_, encodedKeys, err := c.Updates.effectiveKeyring()
	if err != nil {
		return ""
	}
	feed := c.Updates.effectiveFeed()
	if c.Updates.GitHub != "" {
		feed = c.updateFeed(target)
	}
	ch := c.Updates.effectiveChannel(c.Version)
	return " -X " + ldflagsQuote("github.com/egoist/mygo.packageUpdateFeed="+feed) +
		" -X " + ldflagsQuote("github.com/egoist/mygo.packageUpdateKeys="+encodedKeys) +
		" -X " + ldflagsQuote("github.com/egoist/mygo.packageUpdateTarget="+target) +
		" -X " + ldflagsQuote("github.com/egoist/mygo.packageUpdateChannel="+ch)
}

func (c *Config) signingKeys() ([]ed25519.PrivateKey, error) {
	text := os.Getenv("MYGO_UPDATER_PRIVATE_KEY")
	if text == "" && c.Updates.PrivateKey != "" {
		path := c.Updates.PrivateKey
		if rest, ok := strings.CutPrefix(path, "~/"); ok {
			home, _ := os.UserHomeDir()
			path = filepath.Join(home, rest)
		}
		b, err := os.ReadFile(c.path(path))
		if err != nil {
			return nil, err
		}
		text = string(b)
	}
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	keys, err := update.ParsePrivateKeys(text)
	if err != nil {
		return nil, err
	}
	ring, _, err := c.Updates.effectiveKeyring()
	if err != nil {
		return nil, err
	}
	for _, k := range keys {
		kid := update.KeyID(k.Public().(ed25519.PublicKey))
		if !slices.ContainsFunc(ring, func(t update.TrustedKey) bool { return t.ID == kid }) {
			return nil, fmt.Errorf("the update signing key %s does not match updates.publicKey/publicKeys in %s", kid, c.configName())
		}
	}
	return keys, nil
}

func (c *Config) releaseNotes() (string, map[string]string, error) {
	name := c.Updates.Changelog
	if name == "" {
		switch {
		case fileExists(c.path("CHANGELOG.md")):
			name = "CHANGELOG.md"
		default:
			if st, err := os.Stat(c.path("changelogs")); err == nil && st.IsDir() {
				name = "changelogs"
			} else {
				return "", nil, nil
			}
		}
	}
	root := c.path(name)
	info, err := os.Stat(root)
	if err != nil {
		return "", nil, err
	}
	if info.IsDir() {
		notes, err := readReleaseNotesFile(filepath.Join(root, "en.md"), filepath.Join(name, "en.md"), c.Version)
		if err != nil {
			return "", nil, err
		}
		return notes, readLocalizedReleaseNotes(root, "", c.Version), nil
	}
	notes, err := readReleaseNotesFile(root, name, c.Version)
	if err != nil {
		return "", nil, err
	}
	stem := strings.TrimSuffix(filepath.Base(root), filepath.Ext(root)) + "."
	return notes, readLocalizedReleaseNotes(filepath.Dir(root), stem, c.Version), nil
}

func readReleaseNotesFile(path, display, version string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	notes := update.ReleaseNotes(string(data), version)
	if notes == "" {
		return "", fmt.Errorf("%s has no notes for %s: add a \"## %s\" or \"### %s\" section", display, version, version, version)
	}
	return notes, nil
}

func readLocalizedReleaseNotes(dir, stemPrefix, version string) map[string]string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var byLang map[string]string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".md") || (stemPrefix != "" && !strings.HasPrefix(name, stemPrefix)) {
			continue
		}
		lang := strings.TrimSuffix(strings.TrimPrefix(name, stemPrefix), ".md")
		if lang == "" || strings.EqualFold(lang, "en") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		if notes := update.ReleaseNotes(string(data), version); notes != "" {
			if byLang == nil {
				byLang = make(map[string]string)
			}
			byLang[lang] = notes
		}
	}
	return byLang
}

// writeArchive archives stage entries: .tar.zst + SignedManifest (+ v2 deltas +
// history-<target>.json) when configured with signing keys, and .tar.gz on
// Linux for PKGBUILD and install.sh.
func writeArchive(c *Config, stage, target string, entries []string) ([]string, error) {
	var keys []ed25519.PrivateKey
	var ring update.Keyring
	if c.Updates != nil {
		var err error
		if keys, err = c.signingKeys(); err != nil {
			return nil, fmt.Errorf("updates: %w", err)
		}
		if len(keys) == 0 {
			warnf("Not signing an update: set MYGO_UPDATER_PRIVATE_KEY or updates.privateKey")
		} else {
			ring, _, _ = c.Updates.effectiveKeyring()
		}
	}
	isLinux := strings.HasPrefix(target, "linux-")
	if len(keys) == 0 && !isLinux {
		return nil, nil
	}
	if len(keys) > 0 && strings.HasPrefix(target, "darwin-") && c.leavesAppUnsigned() {
		warnUnsignedUpdate(c, target, entries)
	}

	var files []string
	var legacy *update.Artifact
	if isLinux {
		gzName := c.slug() + "-" + c.Version + "-" + target + ".tar.gz"
		gzPath := filepath.Join(stage, gzName)
		var signKey ed25519.PrivateKey
		if len(keys) > 0 {
			signKey = keys[0]
		}
		gzSize, gzSum, gzSig, err := writeSigned(gzPath, signKey, func(w io.Writer) error {
			return update.WriteTarGZ(w, stage, entries)
		})
		if err != nil {
			return nil, err
		}
		files = append(files, gzPath)
		if len(keys) > 0 {
			u, extra := c.artifactURLs(gzName)
			legacy = &update.Artifact{URL: u, URLs: extra, Size: gzSize, SHA256: gzSum, Signature: gzSig}
		}
	}
	if len(keys) == 0 {
		return files, nil
	}

	notes, notesByLang, err := c.releaseNotes()
	if err != nil {
		return nil, err
	}
	zstName := c.slug() + "-" + c.Version + "-" + target + ".tar.zst"
	zstPath := filepath.Join(stage, zstName)
	size, sumHex, sig, err := writeSigned(zstPath, keys[0], func(w io.Writer) error {
		return update.WriteArchive(w, stage, entries)
	})
	if err != nil {
		return nil, err
	}
	files = append(files, zstPath)

	primaryURL, extraURLs := c.artifactURLs(zstName)
	channel := c.Updates.effectiveChannel(c.Version)
	var minSys string
	if strings.HasPrefix(target, "darwin-") {
		minSys = cmp.Or(c.MacOS.MinimumSystemVersion, "12.0")
	}
	m := update.Manifest{
		Schema:        update.SchemaVersion,
		Target:        target,
		Channel:       channel,
		Version:       c.Version,
		Date:          time.Now().UTC().Format(time.RFC3339),
		MinSystem:     minSys,
		Notes:         notes,
		NotesByLang:   notesByLang,
		Mirrors:       slices.Clone(c.Updates.Mirrors),
		LegacyArchive: legacy,
		Artifact: update.Artifact{
			URL:       primaryURL,
			URLs:      extraURLs,
			Size:      size,
			SHA256:    sumHex,
			Signature: sig,
		},
	}

	var history []update.Archive
	if n := c.Updates.deltas(); n > 0 {
		deltas, prevHistory, err := writeDeltas(c, keys[0], ring, stage, target, channel, entries, &m, n)
		if err != nil {
			return nil, err
		}
		files = append(files, deltas...)
		history = prevHistory
	}

	manifestData, err := update.SignManifest(keys, &m)
	if err != nil {
		return nil, err
	}
	manifestPath := filepath.Join(stage, update.ManifestName(target))
	if err := os.WriteFile(manifestPath, manifestData, 0o644); err != nil {
		return nil, err
	}
	files = append(files, manifestPath)

	// Record CI-only delta history separately from client manifests.
	if c.Updates.deltas() > 0 {
		newHist := append([]update.Archive{{
			Version:   c.Version,
			URL:       primaryURL,
			URLs:      extraURLs,
			Size:      size,
			SHA256:    sumHex,
			Signature: sig,
		}}, history...)
		if n := c.Updates.deltas(); len(newHist) > n {
			newHist = newHist[:n]
		}
		histBytes, _ := json.MarshalIndent(update.ReleaseIndex{Target: target, Channel: channel, History: newHist}, "", "  ")
		histPath := filepath.Join(stage, update.HistoryName(target))
		if err := os.WriteFile(histPath, append(histBytes, '\n'), 0o644); err != nil {
			return nil, err
		}
		files = append(files, histPath)
	}
	return files, nil
}

func warnUnsignedUpdate(c *Config, target string, entries []string) {
	app := c.executableName() + ".app"
	if len(entries) > 0 {
		app = entries[0]
	}
	warnf("%s", bold("The update for "+target+" holds "+app+" unsigned: installed copies that update to it will refuse to open")+"\n"+
		"Code signing needs macOS, and this build runs on "+buildHost+", so signing the app afterwards leaves the update unsigned.\n"+
		"Set macos.signCommand to a command that signs the app, e.g. with rcodesign, and mygo build signs it before writing the update:\n"+
		`  "macos": { "signCommand": "scripts/sign-macos.sh %1" }`)
}

func writeSigned(path string, key ed25519.PrivateKey, write func(io.Writer) error) (int64, string, string, error) {
	f, err := os.Create(path)
	if err != nil {
		return 0, "", "", err
	}
	sum := sha256.New()
	err = write(io.MultiWriter(f, sum))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return 0, "", "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0, "", "", err
	}
	digest := sum.Sum(nil)
	hexSum := hex.EncodeToString(digest)
	if key == nil {
		return info.Size(), hexSum, "", nil
	}
	return info.Size(), hexSum, update.SignArtifact(key, digest), nil
}

func writeDeltas(c *Config, key ed25519.PrivateKey, ring update.Keyring, stage, target, channel string, entries []string, m *update.Manifest, n int) ([]string, []update.Archive, error) {
	get := c.archiveGetter(channel)
	bases, err := fetchDeltaBases(c, get, ring, target, channel)
	if err != nil {
		if errors.Is(err, errNotPublished) {
			logf("No delta updates for %s: no version is published yet", target)
		} else {
			warnf("No delta updates for %s: %v", target, err)
		}
		return nil, nil, nil
	}
	var filtered []update.Archive
	for _, a := range bases {
		if update.Compare(a.Version, c.Version) < 0 && !slices.ContainsFunc(filtered, func(b update.Archive) bool { return b.Version == a.Version }) {
			filtered = append(filtered, a)
		}
	}
	if len(filtered) == 0 {
		logf("No delta updates for %s: no older published version", target)
		return nil, nil, nil
	}
	filtered = filtered[:min(len(filtered), n)]

	work, err := os.MkdirTemp(stage, ".delta-")
	if err != nil {
		return nil, nil, err
	}
	defer os.RemoveAll(work)
	newDir, newEntries := stage, entries
	if strings.HasPrefix(target, "darwin-") {
		newDir, newEntries = filepath.Join(stage, entries[0]), nil
	}
	var files []string
	for i, base := range filtered {
		name := c.slug() + "-" + base.Version + "-to-" + c.Version + "-" + target + ".delta"
		path := filepath.Join(stage, name)
		d, err := writeDelta(get, key, ring, base, filepath.Join(work, strconv.Itoa(i)), path, c.Version, newDir, newEntries)
		switch {
		case err != nil:
			warnf("No delta update from %s for %s: %v", base.Version, target, err)
			continue
		case d.Size >= m.Artifact.Size:
			os.Remove(path)
			logf("No delta update from %s for %s: it is not smaller than the archive", base.Version, target)
			continue
		}
		d.URL, d.URLs = c.artifactURLs(name)
		m.Deltas = append(m.Deltas, d)
		files = append(files, path)
		logf("Made the delta update from %s for %s: %s, instead of %s", base.Version, target, formatSize(d.Size), formatSize(m.Artifact.Size))
	}
	return files, filtered, nil
}

func fetchDeltaBases(c *Config, get archiveGetter, ring update.Keyring, target, channel string) ([]update.Archive, error) {
	if histURL := c.historyFeed(target, channel); histURL != "" {
		resp, err := get(context.Background(), histURL)
		if err == nil {
			var idx update.ReleaseIndex
			decErr := json.NewDecoder(io.LimitReader(resp.Body, update.MaxManifestSize)).Decode(&idx)
			resp.Body.Close()
			if decErr == nil && len(idx.History) > 0 {
				return idx.History, nil
			}
		}
	}
	m, manifestURL, err := fetchDeltaManifest(c, ring, target, channel)
	if err != nil {
		return nil, err
	}
	if strings.HasSuffix(manifestURL, "/"+update.ManifestName(target)) {
		sib := strings.TrimSuffix(manifestURL, update.ManifestName(target)) + update.HistoryName(target)
		if resp, err := get(context.Background(), sib); err == nil {
			var idx update.ReleaseIndex
			decErr := json.NewDecoder(io.LimitReader(resp.Body, update.MaxManifestSize)).Decode(&idx)
			resp.Body.Close()
			if decErr == nil && len(idx.History) > 0 {
				return idx.History, nil
			}
		}
	}
	return []update.Archive{{
		Version:   m.Version,
		URL:       m.Artifact.URL,
		URLs:      m.Artifact.URLs,
		Size:      m.Artifact.Size,
		SHA256:    m.Artifact.SHA256,
		Signature: m.Artifact.Signature,
	}}, nil
}

func writeDelta(get archiveGetter, key ed25519.PrivateKey, ring update.Keyring, base update.Archive, work, path, version, newDir string, newEntries []string) (update.Delta, error) {
	if err := os.MkdirAll(work, 0o755); err != nil {
		return update.Delta{}, err
	}
	archive := filepath.Join(work, "archive.tar")
	urls := base.CandidateURLs(nil)
	var fetchErr error
	for _, u := range urls {
		if err := fetchSigned(get, u, base.Size, base.SHA256, base.Signature, ring, archive); err == nil {
			fetchErr = nil
			break
		} else {
			fetchErr = err
		}
	}
	if fetchErr != nil {
		return update.Delta{}, fetchErr
	}
	f, err := os.Open(archive)
	if err != nil {
		return update.Delta{}, err
	}
	oldDir := filepath.Join(work, "app")
	err = update.ExtractArchive(f, oldDir)
	f.Close()
	if err != nil {
		return update.Delta{}, err
	}
	if newEntries == nil {
		all, _ := os.ReadDir(oldDir)
		if len(all) != 1 || !strings.HasSuffix(all[0].Name(), ".app") {
			return update.Delta{}, errors.New("its archive holds no app bundle")
		}
		oldDir = filepath.Join(oldDir, all[0].Name())
	}
	size, sumHex, sig, err := writeSigned(path, key, func(w io.Writer) error {
		return update.WriteDelta(w, base.Version, version, oldDir, newDir, newEntries)
	})
	if err != nil {
		os.Remove(path)
		return update.Delta{}, err
	}
	return update.Delta{From: base.Version, Size: size, SHA256: sumHex, Signature: sig}, nil
}

var errNotPublished = errors.New("not published")

func fetchManifest(feed string, ring update.Keyring) (*update.Manifest, string, error) {
	u, err := update.ResolveFeed(context.Background(), feed, func(_ context.Context, u string) (io.ReadCloser, error) {
		resp, err := updateGet(u, 30*time.Second)
		if err != nil {
			return nil, err
		}
		return resp.Body, nil
	})
	if errors.Is(err, update.ErrNotPublished) {
		return nil, "", errNotPublished
	}
	if err != nil {
		return nil, "", err
	}
	resp, err := updateGet(u, 30*time.Second)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, update.MaxManifestSize+1))
	if err != nil {
		return nil, "", fmt.Errorf("reading %s: %w", u, err)
	}
	if len(raw) > update.MaxManifestSize {
		return nil, "", fmt.Errorf("reading %s: manifest too large", u)
	}
	m, err := update.VerifyManifest(ring, raw)
	if err != nil {
		return nil, "", fmt.Errorf("reading %s: %w", u, err)
	}
	return m, u, nil
}

func fetchDeltaManifest(c *Config, ring update.Keyring, target, channel string) (*update.Manifest, string, error) {
	published, u, err := fetchManifest(c.updateFeedFor(target, channel), ring)
	if !errors.Is(err, errNotPublished) || c.Updates.GitHub == "" {
		return published, u, err
	}
	name := update.ManifestName(target)
	type asset struct {
		Name string `json:"name"`
	}
	const perPage, maxPages = 100, 10
	for page := 1; page <= maxPages; page++ {
		u := fmt.Sprintf("%s/repos/%s/releases?per_page=%d&page=%d", update.GitHubAPI, c.Updates.GitHub, perPage, page)
		resp, err := updateGet(u, 30*time.Second)
		if err != nil {
			return nil, "", err
		}
		var releases []struct {
			TagName    string  `json:"tag_name"`
			Draft      bool    `json:"draft"`
			Prerelease bool    `json:"prerelease"`
			Assets     []asset `json:"assets"`
		}
		err = json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(&releases)
		resp.Body.Close()
		if err != nil {
			return nil, "", fmt.Errorf("reading %s: %w", u, err)
		}
		for _, r := range releases {
			v, ok := strings.CutPrefix(r.TagName, c.Updates.TagPrefix)
			wantPre := channel == update.PrereleaseChannel
			if r.Draft || (!wantPre && r.Prerelease) || !ok || v == "" || v[0] < '0' || v[0] > '9' || update.Compare(v, c.Version) >= 0 {
				continue
			}
			if !slices.ContainsFunc(r.Assets, func(a asset) bool { return a.Name == name }) {
				continue
			}
			feed := "https://github.com/" + c.Updates.GitHub + "/releases/download/" + url.PathEscape(r.TagName) + "/" + url.PathEscape(name)
			published, _, err := fetchManifest(feed, ring)
			if errors.Is(err, errNotPublished) {
				continue
			}
			if err != nil {
				return nil, "", err
			}
			if published.Version != "" && update.Compare(published.Version, c.Version) < 0 {
				return published, feed, nil
			}
		}
		if len(releases) < perPage {
			break
		}
	}
	return nil, "", errNotPublished
}

type archiveGetter func(ctx context.Context, u string) (*http.Response, error)

func (c *Config) archiveGetter(channel string) archiveGetter {
	get := func(ctx context.Context, u string) (*http.Response, error) {
		return updateGetContext(ctx, u, 0)
	}
	if c.Updates.S3 == nil {
		return get
	}
	bucket, err := newS3Client(c.Updates.S3)
	if err != nil {
		logf("Downloading earlier releases from updates.url, not updates.s3: %v", err)
		return get
	}
	prefix := strings.TrimSuffix(c.Updates.URL, "/") + "/"
	_ = channel
	return func(ctx context.Context, u string) (*http.Response, error) {
		rest, ok := strings.CutPrefix(u, prefix)
		name, err := url.PathUnescape(rest)
		if !ok || err != nil || name == "" {
			return get(ctx, u)
		}
		return bucket.get(ctx, bucket.key(name))
	}
}

var (
	fetchAttempts = 3
	fetchStall    = 30 * time.Second
	fetchTimeout  = 10 * time.Minute
)

var errStalled = errors.New("no data")

func fetchSigned(get archiveGetter, u string, size int64, wantSHA256, signature string, ring update.Keyring, path string) error {
	for attempt := 1; ; attempt++ {
		sum, retry, err := fetchOnce(get, u, size, path)
		switch {
		case err == nil && wantSHA256 != "" && !strings.EqualFold(hex.EncodeToString(sum), wantSHA256):
			return fmt.Errorf("%s checksum mismatch", u)
		case err == nil && signature != "" && update.VerifyArtifactAny(ring, sum, signature) != nil:
			return fmt.Errorf("%s is not signed with a key of updates.publicKey", u)
		case err == nil || !retry || attempt == fetchAttempts:
			return err
		}
		delay := time.Duration(attempt) * time.Second
		warnf("Retrying %s in %s: %v", u, delay, err)
		updateRetryWait(delay)
	}
}

func fetchOnce(get archiveGetter, u string, size int64, path string) ([]byte, bool, error) {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	ctx, stop := context.WithTimeout(ctx, fetchTimeout)
	defer stop()
	resp, err := get(ctx, u)
	if err != nil {
		return nil, false, downloadError(u, false, err)
	}
	defer resp.Body.Close()
	stall := time.AfterFunc(fetchStall, func() { cancel(errStalled) })
	defer stall.Stop()
	stalled := func() bool { return errors.Is(context.Cause(ctx), errStalled) }
	f, err := os.Create(path)
	if err != nil {
		return nil, false, err
	}
	sum := sha256.New()
	body := &progressReader{io.LimitReader(resp.Body, size+1), func() { stall.Reset(fetchStall) }}
	n, err := io.Copy(io.MultiWriter(f, sum), body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	switch {
	case err != nil:
		return nil, stalled() || networkError(ctx, err), downloadError(u, stalled(), err)
	case n != size:
		return nil, false, fmt.Errorf("%s has %d bytes, not %d", u, n, size)
	}
	return sum.Sum(nil), false, nil
}

func downloadError(u string, stalled bool, err error) error {
	if stalled {
		return fmt.Errorf("downloading %s: %w for %v", u, errStalled, fetchStall)
	}
	return fmt.Errorf("downloading %s: %w", u, err)
}

type progressReader struct {
	r        io.Reader
	progress func()
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.progress()
	}
	return n, err
}

var updateRetryWait = time.Sleep

func updateGet(u string, timeout time.Duration) (*http.Response, error) {
	return updateGetContext(context.Background(), u, timeout)
}

func updateGetContext(ctx context.Context, u string, timeout time.Duration) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "mygo/"+version)
	if token := cmp.Or(os.Getenv("GH_TOKEN"), os.Getenv("GITHUB_TOKEN")); token != "" && req.URL.Host == "api.github.com" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := retryGet(&http.Client{Timeout: timeout}, req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusOK {
		return resp, nil
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, errNotPublished
	}
	return nil, fmt.Errorf("GET %s: %s", u, resp.Status)
}

func retryGet(client *http.Client, req *http.Request) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		resp, err := client.Do(req)
		switch {
		case err != nil && !networkError(req.Context(), err):
			return nil, err
		case err == nil && resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode/100 != 5:
			return resp, nil
		case attempt == 2:
			return resp, err
		}
		delay := time.Duration(attempt+1) * time.Second
		if resp != nil {
			after := resp.Header.Get("Retry-After")
			if seconds, err := strconv.ParseInt(after, 10, 64); err == nil && seconds > 0 {
				delay = max(delay, time.Duration(min(seconds, 30))*time.Second)
			} else if at, err := http.ParseTime(after); err == nil {
				delay = max(delay, min(time.Until(at), 30*time.Second))
			}
			resp.Body.Close()
			err = errors.New(resp.Status)
		}
		warnf("Retrying %s in %s: %v", req.URL, delay, err)
		updateRetryWait(delay)
	}
}

func networkError(ctx context.Context, err error) bool {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return false
	}
	cause := err
	if e, ok := errors.AsType[*url.Error](err); ok {
		cause = e.Err
	}
	_, ok := errors.AsType[net.Error](cause)
	return ok || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}

func runKeygen(args []string) error {
	config, _ := os.UserConfigDir()
	flags := newFlags("keygen", "[flags]", `Creates an Ed25519 key pair for signed updates (mygo-update.key and mygo-update.pub).`)
	out := flags.String("o", filepath.Join(config, "mygo", "update-keys"), "directory to write the keys to")
	force := flags.Bool("force", false, "overwrite existing keys")
	if err := flags.Parse(args); err != nil {
		return err
	}
	secret, public := filepath.Join(*out, "mygo-update.key"), filepath.Join(*out, "mygo-update.pub")
	if !*force && (fileExists(secret) || fileExists(public)) {
		return fmt.Errorf("%s already holds keys; pass -force to replace them", *out)
	}
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*out, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(secret, []byte(fmt.Sprintf("%x\n", []byte{}))[:0], 0o600); err != nil {
		return err
	}
	privEnc := strings.TrimSpace(fmt.Sprintf("%s", encodeBase64(priv)))
	pubText := encodeBase64(pub)
	if err := os.WriteFile(secret, []byte(privEnc+"\n"), 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(public, []byte(pubText+"\n"), 0o644); err != nil {
		return err
	}
	kid := update.KeyID(pub)
	logf("Wrote %s (secret) and %s (kid %s)", secret, public, kid)
	fmt.Printf("Key ID: %s\nAdd to mygo.json:\n\n  \"updates\": {\n    \"publicKey\": %q,\n    \"feedUrl\": \"https://updates.example.com/app\",\n    \"url\": \"https://dl.example.com/app\"\n  }\n", kid, pubText)
	return nil
}

func encodeBase64(b []byte) string {
	return update.EncodeKeyring([]ed25519.PublicKey{ed25519.PublicKey(b)})
}
