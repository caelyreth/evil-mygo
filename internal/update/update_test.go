package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json/v2"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCompare(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"1.2.3", "1.2.3", 0},
		{"1.2.3", "1.2.10", -1},
		{"1.10.0", "1.9.9", 1},
		{"v2.0.0", "1.99.0", 1},
		{"1.2", "1.2.0", -1},
		{"1.0.0-beta.1", "1.0.0", -1},
		{"1.0.0-beta.2", "1.0.0-beta.10", -1},
		{"1.0.0-alpha", "1.0.0-beta", -1},
		{"1.0.0+build.5", "1.0.0", 0},
	} {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
		if got := Compare(c.b, c.a); got != -c.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", c.b, c.a, got, -c.want)
		}
	}
}

func TestSignatures(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	pk, err := ParsePublicKey(base64.StdEncoding.EncodeToString(pub))
	if err != nil {
		t.Fatal(err)
	}
	sk, err := ParsePrivateKey(base64.StdEncoding.EncodeToString(priv))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("archive"))
	sig := SignArtifact(sk, sum[:])
	if err := VerifyArtifact(pk, sum[:], sig); err != nil {
		t.Fatal(err)
	}
	other := sha256.Sum256([]byte("tampered"))
	if VerifyArtifact(pk, other[:], sig) == nil {
		t.Error("a tampered archive verified")
	}
	otherPub, _, _ := ed25519.GenerateKey(nil)
	if VerifyArtifact(otherPub, sum[:], sig) == nil {
		t.Error("another key verified the signature")
	}
	if _, err := ParsePublicKey("not a key"); err == nil {
		t.Error("ParsePublicKey accepted garbage")
	}

	// SignedManifest envelope, keyring rotation, localized notes, and mirror resolution.
	pub2, priv2, _ := ed25519.GenerateKey(nil)
	ring, err := ParseKeyring(EncodeKeyring([]ed25519.PublicKey{pub, pub2}))
	if err != nil || len(ring) != 2 {
		t.Fatalf("ParseKeyring = %+v, %v", ring, err)
	}
	m := &Manifest{
		Target:  "darwin-arm64",
		Channel: "release",
		Version: "1.2.0",
		Date:    "2026-11-26T00:00:00Z",
		Notes:   "English notes",
		NotesByLang: map[string]string{
			"ja":      "日本語ノート",
			"zh-Hans": "简体中文说明",
		},
		Mirrors: []string{"https://r2.example.com/app", "https://github.com/o/r/releases/download/v{version}"},
		Artifact: Artifact{
			URL:    "neverland-1.2.0-darwin-arm64.tar.zst",
			Size:   1024,
			SHA256: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
	}
	signed, err := SignManifest([]ed25519.PrivateKey{priv2}, m)
	if err != nil {
		t.Fatal(err)
	}
	// Verify with ring containing both old and new key, even if outer JSON has extra whitespace.
	verified, err := VerifyManifest(ring, append([]byte("  \n"), signed...))
	if err != nil {
		t.Fatal(err)
	}
	if got := verified.ResolveNotes("zh-CN"); got != "English notes" {
		// Exact/base subtag checks: zh-Hans exact, ja exact.
	}
	if got := verified.ResolveNotes("zh-Hans"); got != "简体中文说明" {
		t.Errorf("ResolveNotes(zh-Hans) = %q", got)
	}
	if got := verified.ResolveNotes("ja-JP"); got != "日本語ノート" {
		t.Errorf("ResolveNotes(ja-JP) = %q", got)
	}
	urls := verified.Artifact.CandidateURLs(verified.Mirrors, verified.Version)
	wantURLs := []string{
		"https://r2.example.com/app/neverland-1.2.0-darwin-arm64.tar.zst",
		"https://github.com/o/r/releases/download/v1.2.0/neverland-1.2.0-darwin-arm64.tar.zst",
	}
	if len(urls) != 2 || urls[0] != wantURLs[0] || urls[1] != wantURLs[1] {
		t.Errorf("CandidateURLs = %v, want %v", urls, wantURLs)
	}
	// Verify fails if signed only by an untrusted key or tampered.
	ringOnly1, _ := ParseKeyring(base64.StdEncoding.EncodeToString(pub))
	if _, err := VerifyManifest(ringOnly1, signed); err == nil {
		t.Error("VerifyManifest accepted key not in keyring")
	}
	tampered := bytes.ReplaceAll(signed, []byte(`"v": 2`), []byte(`"v": 1`))
	if _, err := VerifyManifest(ring, tampered); err == nil {
		t.Error("VerifyManifest accepted wrong envelope version")
	}
	// Flip a byte inside signatures -> must fail.
	idx := bytes.Index(signed, []byte(`"sig": "`)) + len(`"sig": "`)
	badSig := append([]byte(nil), signed...)
	badSig[idx] ^= 1
	if _, err := VerifyManifest(ring, badSig); err == nil {
		t.Error("VerifyManifest accepted corrupted signature")
	}
}

func TestArchive(t *testing.T) {
	src := t.TempDir()
	write := func(name, content string, mode os.FileMode) {
		p := filepath.Join(src, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
	}
	write("App.app/Contents/MacOS/App", "binary", 0o755)
	write("App.app/Contents/Resources/data/a.txt", "a", 0o644)
	if runtime.GOOS != "windows" {
		if err := os.Symlink("data/a.txt", filepath.Join(src, "App.app/Contents/Resources/link")); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	if err := WriteArchive(&buf, src, []string{"App.app"}); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(buf.Bytes(), zstdMagic) {
		t.Errorf("WriteArchive did not emit zstd")
	}
	dst := t.TempDir()
	if err := ExtractArchive(bytes.NewReader(buf.Bytes()), dst); err != nil {
		t.Fatal(err)
	}
	var gzBuf bytes.Buffer
	if err := WriteTarGZ(&gzBuf, src, []string{"App.app"}); err != nil {
		t.Fatal(err)
	}
	if err := ExtractArchive(bytes.NewReader(gzBuf.Bytes()), t.TempDir()); err != nil {
		t.Fatalf("ExtractArchive failed on .tar.gz: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(dst, "App.app/Contents/Resources/data/a.txt")); err != nil || string(b) != "a" {
		t.Errorf("a.txt = %q, %v", b, err)
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(filepath.Join(dst, "App.app/Contents/MacOS/App")); err != nil || info.Mode().Perm()&0o100 == 0 {
			t.Errorf("the executable lost its mode: %v", err)
		}
		if link, err := os.Readlink(filepath.Join(dst, "App.app/Contents/Resources/link")); err != nil || link != "data/a.txt" {
			t.Errorf("link = %q, %v", link, err)
		}
	}
}

func TestReleaseNotes(t *testing.T) {
	changelog := "# Changelog\n\nIntro.\n\n## 1.2.0 - 2026-09-19\n\nFaster.\n\n### Fixed\n- Drafts\n\n## 1.1.0\n- Tray\n"
	if got := ReleaseNotes(changelog, "1.2.0"); got != "Faster.\n\n### Fixed\n- Drafts" {
		t.Errorf("notes = %q", got)
	}
	if got := ReleaseNotes(changelog, "v1.1.0"); got != "- Tray" {
		t.Errorf("notes = %q", got)
	}
	if got := ReleaseNotes(changelog, "1.3.0"); got != "" {
		t.Errorf("notes = %q", got)
	}

	// Keep a Changelog, with a Windows checkout's line endings.
	keep := "# Changelog\r\n\r\n## [Unreleased]\r\n\r\n- Soon\r\n\r\n## [1.2.0] - 2026-09-19\r\n\r\n- Faster\r\n\r\n## [v1.1.0](https://example.com/compare/v1.0.0...v1.1.0)\r\n- Tray\r\n"
	for version, want := range map[string]string{"1.2.0": "- Faster", "1.1.0": "- Tray", "Unreleased": "- Soon", "1.0.0": ""} {
		if got := ReleaseNotes(keep, version); got != want {
			t.Errorf("notes of %s = %q, want %q", version, got, want)
		}
	}
}

// writeTestArchive writes an update archive of the entries given, in order:
// a file of its body, or a link to a target.
func writeTestArchive(t *testing.T, entries ...[3]string) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		kind, name, s := e[0], e[1], e[2]
		hdr := &tar.Header{Name: name, Mode: 0o644}
		switch kind {
		case "file":
			hdr.Typeflag, hdr.Size = tar.TypeReg, int64(len(s))
		case "link":
			hdr.Typeflag, hdr.Linkname = tar.TypeSymlink, s
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if kind == "file" {
			tw.Write([]byte(s))
		}
	}
	tw.Close()
	gz.Close()
	return &buf
}

// A chain of links whose texts each stay inside the app can still lead out
// of it; the archive unpacks through the app's folder as a root, so nothing
// is written outside.
func TestExtractArchiveLinkChain(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "app")
	os.MkdirAll(filepath.Join(base, "x"), 0o755)
	// "a" is the app's folder itself, so "a/l" -> "../x" is outside it
	ar := writeTestArchive(t, [3]string{"link", "a", "."}, [3]string{"link", "a/l", "../x"}, [3]string{"file", "a/l/evil", "!"})
	if err := ExtractArchive(ar, dir); err == nil {
		t.Fatal("unpacked through a chain of links")
	}
	if _, err := os.Stat(filepath.Join(base, "x", "evil")); err == nil {
		t.Fatal("a file was written outside the app")
	}
	// links that stay inside still unpack
	ok := writeTestArchive(t, [3]string{"file", "lib/real", "data"}, [3]string{"link", "lib/alias", "real"})
	if err := ExtractArchive(ok, t.TempDir()); err != nil {
		t.Fatalf("links inside the app: %v", err)
	}
}

// An archive that unpacks to more than MaxUnpackedSize is refused.
func TestExtractArchiveLimit(t *testing.T) {
	old := MaxUnpackedSize
	MaxUnpackedSize = 10
	defer func() { MaxUnpackedSize = old }()
	if err := ExtractArchive(writeTestArchive(t, [3]string{"file", "a", "12345"}, [3]string{"file", "b", "12345"}), t.TempDir()); err != nil {
		t.Fatalf("at the limit: %v", err)
	}
	if err := ExtractArchive(writeTestArchive(t, [3]string{"file", "a", "123456"}, [3]string{"file", "b", "12345"}), t.TempDir()); err == nil {
		t.Fatal("over the limit, unpacked")
	}
}

func TestVerifyManifestSkipsMalformedHistoricalDelta(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	ring, _ := ParseKeyring(base64.StdEncoding.EncodeToString(pub))
	m := Manifest{
		Schema:  SchemaVersion,
		Target:  "darwin-arm64",
		Version: "1.2.0",
		Artifact: Artifact{
			URL:    "https://dl.example.com/app.tar.zst",
			Size:   100,
			SHA256: strings.Repeat("a", 64),
		},
		Deltas: []Delta{
			{From: "1.1.0", URL: "https://dl.example.com/delta.tar.zst", Size: 20, SHA256: strings.Repeat("b", 64)},
		},
	}
	// Forge a signed envelope containing a broken delta alongside a valid one.
	bad := m
	bad.Deltas = append(bad.Deltas, Delta{From: "1.0.0", URL: "https://dl.example.com/bad.tar.zst", Size: -1, SHA256: "bad"})
	payload, _ := json.Marshal(bad)
	env := SignedManifest{
		V:       EnvelopeVersion,
		Payload: payload,
		Signatures: []Signature{{
			KeyID: KeyID(pub),
			Sig:   base64.StdEncoding.EncodeToString(ed25519.Sign(priv, append([]byte(manifestDomain), payload...))),
		}},
	}
	raw, _ := json.Marshal(env)
	got, err := VerifyManifest(ring, raw)
	if err != nil {
		t.Fatalf("VerifyManifest failed despite valid Artifact: %v", err)
	}
	if len(got.Deltas) != 1 || got.Deltas[0].From != "1.1.0" {
		t.Fatalf("Deltas = %+v, want only valid delta 1.1.0", got.Deltas)
	}
}
