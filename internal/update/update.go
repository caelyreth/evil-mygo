// Package update holds what the updater of package mygo and `mygo build`
// share: signed update manifests, keyrings, versions, and zstd/tar
// archives and delta updates.
package update

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/klauspost/compress/zstd"
)

const (
	// EnvelopeVersion is the signed manifest wrapper format version.
	EnvelopeVersion = 2
	// SchemaVersion is the Manifest payload schema version.
	SchemaVersion = 2

	// DefaultChannel is the release channel used when none is configured.
	DefaultChannel = "release"
	// PrereleaseChannel is the channel used for prerelease builds.
	PrereleaseChannel = "prerelease"

	manifestDomain = "app-update-manifest-v2\n"
	artifactDomain = "app-update-artifact-v2\n"
)

// Limits of what an app downloads.
const (
	MaxManifestSize = 1 << 20
	MaxArchiveSize  = 1 << 30
)

// SignedManifest is the signed update envelope published as update-<target>.json.
// Payload holds the exact UTF-8 JSON bytes of Manifest so proxies or JSON
// pretty-printers cannot alter whitespace and invalidate signatures.
type SignedManifest struct {
	V          int         `json:"v"`
	Payload    []byte      `json:"payload"`
	Signatures []Signature `json:"signatures"`
}

// Signature authenticates SignedManifest.Payload using an Ed25519 key
// identified by KeyID.
type Signature struct {
	KeyID string `json:"kid"`
	Sig   string `json:"sig"`
}

// Manifest describes an available update for one target and channel.
// It is always decoded from a verified SignedManifest envelope.
type Manifest struct {
	Schema     int    `json:"schema"`
	Target     string `json:"target"`
	Channel    string `json:"channel,omitempty"`
	Version    string `json:"version"`
	Sequence   uint64 `json:"sequence,omitempty"`
	Date       string `json:"date,omitempty"`
	MinSystem  string `json:"minSystem,omitempty"`
	MinVersion string `json:"minVersion,omitempty"`
	// Notes are default release notes in Markdown, with optional localized
	// overrides keyed by BCP 47 tag in NotesByLang.
	Notes       string            `json:"notes,omitempty"`
	NotesByLang map[string]string `json:"notesByLang,omitempty"`
	// Mirrors holds ordered base URLs (with optional {version} placeholders)
	// used to derive fallback download locations or resolve relative paths.
	Mirrors       []string  `json:"mirrors,omitempty"`
	Artifact      Artifact  `json:"artifact"`
	LegacyArchive *Artifact `json:"legacyArchive,omitempty"`
	Deltas        []Delta   `json:"deltas,omitempty"`
}

// Artifact describes a downloadable update archive (normally .tar.zst).
type Artifact struct {
	URL       string   `json:"url"`
	URLs      []string `json:"urls,omitempty"`
	Size      int64    `json:"size"`
	SHA256    string   `json:"sha256"`
	Signature string   `json:"signature,omitempty"`
}

// Delta describes a delta patch from version From to Manifest.Version.
type Delta struct {
	From      string   `json:"from"`
	URL       string   `json:"url"`
	URLs      []string `json:"urls,omitempty"`
	Size      int64    `json:"size"`
	SHA256    string   `json:"sha256"`
	Signature string   `json:"signature,omitempty"`
}

// ReleaseIndex stores build-time delta history separately from the public
// client manifest.
type ReleaseIndex struct {
	Target  string    `json:"target"`
	Channel string    `json:"channel,omitempty"`
	History []Archive `json:"history,omitempty"`
}

// Archive describes an earlier release archive retained for delta generation.
type Archive struct {
	Version   string   `json:"version"`
	URL       string   `json:"url"`
	URLs      []string `json:"urls,omitempty"`
	Size      int64    `json:"size"`
	SHA256    string   `json:"sha256"`
	Signature string   `json:"signature,omitempty"`
}

// Delta returns the delta update for version from, or nil.
func (m *Manifest) Delta(from string) *Delta {
	for i := range m.Deltas {
		if m.Deltas[i].From == from {
			return &m.Deltas[i]
		}
	}
	return nil
}

// ResolveNotes returns localized release notes for lang, falling back to
// base language subtags and then m.Notes.
func (m *Manifest) ResolveNotes(lang string) string {
	if m == nil {
		return ""
	}
	if len(m.NotesByLang) > 0 && lang != "" {
		norm := normalizeLang(lang)
		for k, v := range m.NotesByLang {
			if normalizeLang(k) == norm && v != "" {
				return v
			}
		}
		if base, _, ok := strings.Cut(norm, "-"); ok {
			for k, v := range m.NotesByLang {
				if normalizeLang(k) == base && v != "" {
					return v
				}
			}
		}
	}
	return m.Notes
}

func normalizeLang(tag string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(tag), "_", "-"))
}

// CandidateURLs returns ordered, deduplicated download URLs for the artifact,
// expanding relative paths and {version} templates across mirrors.
func (a Artifact) CandidateURLs(mirrors []string, version string) []string {
	return collectCandidateURLs(a.URL, a.URLs, mirrors, version)
}

// CandidateURLs returns ordered, deduplicated download URLs for the delta,
// expanding relative paths and {version} templates across mirrors.
func (d Delta) CandidateURLs(mirrors []string, version string) []string {
	return collectCandidateURLs(d.URL, d.URLs, mirrors, version)
}

// CandidateURLs returns ordered, deduplicated download URLs for an earlier archive.
func (a Archive) CandidateURLs(mirrors []string) []string {
	return collectCandidateURLs(a.URL, a.URLs, mirrors, a.Version)
}

func collectCandidateURLs(primary string, extra, mirrors []string, version string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" || seen[raw] {
			return
		}
		seen[raw] = true
		out = append(out, raw)
	}
	expand := func(base, rel string) string {
		if version != "" {
			base = strings.ReplaceAll(base, VersionPlaceholder, url.PathEscape(version))
		}
		base = strings.TrimSuffix(base, "/")
		rel = strings.TrimPrefix(rel, "/")
		return base + "/" + rel
	}
	for _, candidate := range append([]string{primary}, extra...) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if strings.Contains(candidate, "://") || len(mirrors) == 0 {
			add(candidate)
			continue
		}
		for _, mirror := range mirrors {
			add(expand(mirror, candidate))
		}
	}
	return out
}

// ManifestName is the file name of the manifest of target.
func ManifestName(target string) string { return "update-" + target + ".json" }

// ManifestPath is the channel-qualified relative path of a manifest.
func ManifestPath(channel, target string) string {
	if channel == "" {
		channel = DefaultChannel
	}
	return channel + "/" + ManifestName(target)
}

// HistoryName is the file name of the CI delta history of target.
func HistoryName(target string) string { return "history-" + target + ".json" }

// HistoryPath is the channel-qualified relative path of the CI delta history.
func HistoryPath(channel, target string) string {
	if channel == "" {
		channel = DefaultChannel
	}
	return channel + "/" + HistoryName(target)
}

// KeyID returns the short key identifier (16 lowercase hex chars of SHA-256)
// for pub.
func KeyID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:8])
}

// TrustedKey pairs an Ed25519 public key with its KeyID.
type TrustedKey struct {
	ID  string
	Key ed25519.PublicKey
}

// Keyring is a set of trusted public keys used to verify manifests and artifacts.
type Keyring []TrustedKey

// ParsePublicKey decodes a base64 Ed25519 public key.
func ParsePublicKey(s string) (ed25519.PublicKey, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, errors.New("invalid update public key")
	}
	return ed25519.PublicKey(b), nil
}

// ParseKeyring parses a single base64 key, a comma/whitespace-separated list
// of base64 keys, or a JSON array of base64 keys into a Keyring.
func ParseKeyring(raw string) (Keyring, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, errors.New("empty update keyring")
	}
	var tokens []string
	if strings.HasPrefix(s, "[") {
		if err := json.Unmarshal([]byte(s), &tokens); err != nil {
			return nil, fmt.Errorf("invalid update keyring: %w", err)
		}
	} else {
		tokens = strings.FieldsFunc(s, func(r rune) bool {
			return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t' || r == ' '
		})
	}
	if len(tokens) == 0 {
		return nil, errors.New("empty update keyring")
	}
	ring := make(Keyring, 0, len(tokens))
	seen := map[string]bool{}
	for _, tok := range tokens {
		pub, err := ParsePublicKey(tok)
		if err != nil {
			return nil, err
		}
		id := KeyID(pub)
		if !seen[id] {
			seen[id] = true
			ring = append(ring, TrustedKey{ID: id, Key: pub})
		}
	}
	return ring, nil
}

// EncodeKeyring encodes keys as a comma-separated list of base64 public keys.
func EncodeKeyring(keys []ed25519.PublicKey) string {
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = base64.StdEncoding.EncodeToString(k)
	}
	return strings.Join(out, ",")
}

// ParsePrivateKey decodes a base64 Ed25519 private key.
func ParsePrivateKey(s string) (ed25519.PrivateKey, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil || len(b) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid update private key")
	}
	return ed25519.PrivateKey(b), nil
}

// ParsePrivateKeys decodes one or more base64 Ed25519 private keys separated
// by commas, semicolons, or whitespace, or encoded as a JSON array.
func ParsePrivateKeys(raw string) ([]ed25519.PrivateKey, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, errors.New("empty update private keys")
	}
	var tokens []string
	if strings.HasPrefix(s, "[") {
		if err := json.Unmarshal([]byte(s), &tokens); err != nil {
			return nil, fmt.Errorf("invalid update private keys: %w", err)
		}
	} else {
		tokens = strings.FieldsFunc(s, func(r rune) bool {
			return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t' || r == ' '
		})
	}
	var keys []ed25519.PrivateKey
	for _, tok := range tokens {
		k, err := ParsePrivateKey(tok)
		if err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, nil
}

// SignArtifact signs the SHA-256 digest of an archive or delta with domain separation.
func SignArtifact(key ed25519.PrivateKey, digest []byte) string {
	msg := make([]byte, len(artifactDomain)+len(digest))
	copy(msg, artifactDomain)
	copy(msg[len(artifactDomain):], digest)
	return base64.StdEncoding.EncodeToString(ed25519.Sign(key, msg))
}

// VerifyArtifact checks a standalone artifact signature against key.
func VerifyArtifact(key ed25519.PublicKey, digest []byte, signature string) error {
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return errors.New("the update is not signed with the app's key")
	}
	msg := make([]byte, len(artifactDomain)+len(digest))
	copy(msg, artifactDomain)
	copy(msg[len(artifactDomain):], digest)
	if !ed25519.Verify(key, msg, sig) {
		return errors.New("the update is not signed with the app's key")
	}
	return nil
}

// VerifyArtifactAny checks a standalone artifact signature against any key in ring.
func VerifyArtifactAny(ring Keyring, digest []byte, signature string) error {
	for _, tk := range ring {
		if VerifyArtifact(tk.Key, digest, signature) == nil {
			return nil
		}
	}
	return errors.New("the update is not signed with a trusted key")
}

// SignManifest serializes m and signs it with each private key in keys,
// returning the JSON-encoded SignedManifest envelope.
func SignManifest(keys []ed25519.PrivateKey, m *Manifest) ([]byte, error) {
	if len(keys) == 0 {
		return nil, errors.New("no signing keys provided")
	}
	if m == nil {
		return nil, errors.New("nil manifest")
	}
	copyM := *m
	copyM.Schema = SchemaVersion
	if copyM.Channel == "" {
		copyM.Channel = DefaultChannel
	}
	if err := validateManifest(&copyM); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(copyM)
	if err != nil {
		return nil, err
	}
	msg := make([]byte, len(manifestDomain)+len(payload))
	copy(msg, manifestDomain)
	copy(msg[len(manifestDomain):], payload)

	sigs := make([]Signature, 0, len(keys))
	seen := map[string]bool{}
	for _, k := range keys {
		pub, ok := k.Public().(ed25519.PublicKey)
		if !ok {
			return nil, errors.New("invalid Ed25519 signing key")
		}
		kid := KeyID(pub)
		if seen[kid] {
			continue
		}
		seen[kid] = true
		sigs = append(sigs, Signature{
			KeyID: kid,
			Sig:   base64.StdEncoding.EncodeToString(ed25519.Sign(k, msg)),
		})
	}
	env := SignedManifest{V: EnvelopeVersion, Payload: payload, Signatures: sigs}
	data, err := json.Marshal(env)
	if err != nil {
		return nil, err
	}
	var pretty bytes.Buffer
	if err := indentJSON(&pretty, data); err == nil {
		pretty.WriteByte('\n')
		return pretty.Bytes(), nil
	}
	return append(data, '\n'), nil
}

// indentJSON formats JSON cleanly without external helper complexity.
func indentJSON(dst *bytes.Buffer, src []byte) error {
	var v any
	if err := json.Unmarshal(src, &v); err != nil {
		return err
	}
	b, err := marshalIndent(v)
	if err != nil {
		return err
	}
	dst.Write(b)
	return nil
}

func marshalIndent(v any) ([]byte, error) {
	// Marshal back with standard indentation using SignedManifest structure or raw formatting.
	// Note: Payload is base64 in SignedManifest so indenting the outer JSON never affects signatures.
	var env SignedManifest
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return raw, nil
	}
	var b bytes.Buffer
	b.WriteString("{\n  \"v\": ")
	b.WriteString(strconv.Itoa(env.V))
	b.WriteString(",\n  \"payload\": ")
	p, _ := json.Marshal(env.Payload)
	b.Write(p)
	b.WriteString(",\n  \"signatures\": [\n")
	for i, s := range env.Signatures {
		if i > 0 {
			b.WriteString(",\n")
		}
		k, _ := json.Marshal(s.KeyID)
		sg, _ := json.Marshal(s.Sig)
		b.WriteString("    {\"kid\": ")
		b.Write(k)
		b.WriteString(", \"sig\": ")
		b.Write(sg)
		b.WriteByte('}')
	}
	b.WriteString("\n  ]\n}")
	return b.Bytes(), nil
}

// VerifyManifest verifies a SignedManifest JSON document against ring before
// decoding and validating the inner Manifest.
func VerifyManifest(ring Keyring, raw []byte) (*Manifest, error) {
	if len(ring) == 0 {
		return nil, errors.New("no trusted update keys configured")
	}
	var env SignedManifest
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("invalid manifest envelope: %w", err)
	}
	if env.V != EnvelopeVersion {
		return nil, fmt.Errorf("unsupported manifest version %d", env.V)
	}
	if len(env.Payload) == 0 || len(env.Signatures) == 0 {
		return nil, errors.New("incomplete signed manifest")
	}
	msg := make([]byte, len(manifestDomain)+len(env.Payload))
	copy(msg, manifestDomain)
	copy(msg[len(manifestDomain):], env.Payload)

	verified := false
	for _, s := range env.Signatures {
		sigBytes, err := base64.StdEncoding.DecodeString(s.Sig)
		if err != nil || len(sigBytes) != ed25519.SignatureSize {
			continue
		}
		for _, tk := range ring {
			if s.KeyID != "" && s.KeyID != tk.ID {
				continue
			}
			if ed25519.Verify(tk.Key, msg, sigBytes) {
				verified = true
				break
			}
		}
		if verified {
			break
		}
	}
	if !verified {
		return nil, errors.New("manifest signature is not trusted")
	}
	var m Manifest
	if err := json.Unmarshal(env.Payload, &m); err != nil {
		return nil, fmt.Errorf("invalid signed manifest payload: %w", err)
	}
	if err := validateVerifiedManifest(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

// validateVerifiedManifest checks mandatory manifest fields and drops any
// malformed historical delta entries so a stale bad delta cannot block full
// archive recovery.
func validateVerifiedManifest(m *Manifest) error {
	if err := validateManifestCore(m); err != nil {
		return err
	}
	if len(m.Deltas) > 0 {
		valid := m.Deltas[:0]
		for _, d := range m.Deltas {
			if d.From == "" {
				continue
			}
			if err := validateDigestSpec("delta "+d.From, d.Size, d.SHA256, d.CandidateURLs(m.Mirrors, m.Version)); err != nil {
				continue
			}
			valid = append(valid, d)
		}
		m.Deltas = valid
	}
	return nil
}

func validateManifestCore(m *Manifest) error {
	if m.Schema != SchemaVersion {
		return fmt.Errorf("unsupported manifest schema %d", m.Schema)
	}
	if m.Version == "" || m.Target == "" {
		return errors.New("incomplete manifest: missing version or target")
	}
	return validateDigestSpec("artifact", m.Artifact.Size, m.Artifact.SHA256, m.Artifact.CandidateURLs(m.Mirrors, m.Version))
}

func validateManifest(m *Manifest) error {
	if err := validateManifestCore(m); err != nil {
		return err
	}
	for _, d := range m.Deltas {
		if d.From == "" {
			return errors.New("delta missing source version")
		}
		if err := validateDigestSpec("delta "+d.From, d.Size, d.SHA256, d.CandidateURLs(m.Mirrors, m.Version)); err != nil {
			return err
		}
	}
	return nil
}

func validateDigestSpec(label string, size int64, sha string, urls []string) error {
	if size <= 0 || size > MaxArchiveSize {
		return fmt.Errorf("%s invalid size %d", label, size)
	}
	if !ValidSHA256(sha) {
		return fmt.Errorf("%s invalid sha256", label)
	}
	if len(urls) == 0 {
		return fmt.Errorf("%s has no download URLs", label)
	}
	return nil
}

// ValidSHA256 reports whether s is a 64-character hex-encoded SHA-256 digest.
func ValidSHA256(s string) bool {
	if len(s) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// Compare compares versions such as 1.2.3 and 1.3.0-beta.1 like semantic
// versioning does: numbers numerically, a pre-release before its release.
func Compare(a, b string) int {
	a, b = strings.TrimPrefix(a, "v"), strings.TrimPrefix(b, "v")
	a, _, _ = strings.Cut(a, "+")
	b, _, _ = strings.Cut(b, "+")
	ar, apre, _ := strings.Cut(a, "-")
	br, bpre, _ := strings.Cut(b, "-")
	if c := compareFields(ar, br); c != 0 {
		return c
	}
	switch {
	case apre == bpre:
		return 0
	case apre == "":
		return 1
	case bpre == "":
		return -1
	}
	return compareFields(apre, bpre)
}

// SystemSatisfied reports whether current satisfies minRequired (or returns true
// if minRequired is empty or current cannot be determined).
func SystemSatisfied(minRequired, current string) bool {
	if strings.TrimSpace(minRequired) == "" || strings.TrimSpace(current) == "" {
		return true
	}
	return Compare(current, minRequired) >= 0
}

func compareFields(a, b string) int {
	af, bf := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(af) || i < len(bf); i++ {
		var x, y string
		if i < len(af) {
			x = af[i]
		}
		if i < len(bf) {
			y = bf[i]
		}
		xn, xerr := strconv.Atoi(x)
		yn, yerr := strconv.Atoi(y)
		switch {
		case x == y:
			continue
		case x == "":
			return -1
		case y == "":
			return 1
		case xerr == nil && yerr == nil:
			if xn < yn {
				return -1
			}
			if xn > yn {
				return 1
			}
		case xerr == nil:
			return -1 // numbers sort before words
		case yerr == nil:
			return 1
		default:
			return strings.Compare(x, y)
		}
	}
	return 0
}

// WriteArchive writes the entries of dir as a zstd-compressed tar archive (.tar.zst).
func WriteArchive(w io.Writer, dir string, entries []string) error {
	zw, err := zstd.NewWriter(w, zstd.WithEncoderLevel(zstd.SpeedBestCompression))
	if err != nil {
		return err
	}
	if err := writeTar(zw, dir, entries); err != nil {
		zw.Close()
		return err
	}
	return zw.Close()
}

// WriteTarGZ writes the entries of dir as a gzip-compressed tar archive (.tar.gz).
func WriteTarGZ(w io.Writer, dir string, entries []string) error {
	gz, err := gzip.NewWriterLevel(w, gzip.BestCompression)
	if err != nil {
		return err
	}
	if err := writeTar(gz, dir, entries); err != nil {
		gz.Close()
		return err
	}
	return gz.Close()
}

func writeTar(w io.Writer, dir string, entries []string) error {
	tw := tar.NewWriter(w)
	for _, entry := range entries {
		err := filepath.WalkDir(filepath.Join(dir, entry), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(dir, p)
			if err != nil {
				return err
			}
			link := ""
			if info.Mode()&fs.ModeSymlink != 0 {
				if link, err = os.Readlink(p); err != nil {
					return err
				}
			}
			hdr, err := tar.FileInfoHeader(info, link)
			if err != nil {
				return err
			}
			hdr.Name = filepath.ToSlash(rel)
			if d.IsDir() {
				hdr.Name += "/"
			}
			hdr.Uid, hdr.Gid, hdr.Uname, hdr.Gname = 0, 0, "", ""
			if info.Mode().IsRegular() {
				f, err := os.Open(p)
				if err != nil {
					return err
				}
				var head [4]byte
				hn, _ := io.ReadFull(f, head[:])
				_, _ = f.Seek(0, io.SeekStart)
				mode := ArchiveFileMode(head[:hn], info.Mode().Perm())
				hdr.Mode = int64(mode)
				if err := tw.WriteHeader(hdr); err != nil {
					f.Close()
					return err
				}
				_, err = io.Copy(tw, f)
				f.Close()
				return err
			}
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return tw.Close()
}

// ArchiveFileMode ensures regular executable binaries/scripts keep Unix +x
// bits even when packaged on hosts (such as Windows) whose filesystems lack
// Unix mode bits.
func ArchiveFileMode(head []byte, perm fs.FileMode) fs.FileMode {
	if perm&0o111 != 0 {
		return perm
	}
	if hasExecutableMagic(head) {
		return perm | 0o755
	}
	return perm
}

func hasExecutableMagic(head []byte) bool {
	if len(head) >= 2 && head[0] == '#' && head[1] == '!' {
		return true
	}
	if len(head) < 4 {
		return false
	}
	// ELF (\x7fELF)
	if head[0] == 0x7f && head[1] == 'E' && head[2] == 'L' && head[3] == 'F' {
		return true
	}
	// Mach-O 32/64/universal magics (big/little endian)
	switch [4]byte(head[:4]) {
	case [4]byte{0xfe, 0xed, 0xfa, 0xce},
		[4]byte{0xce, 0xfa, 0xed, 0xfe},
		[4]byte{0xfe, 0xed, 0xfa, 0xcf},
		[4]byte{0xcf, 0xfa, 0xed, 0xfe},
		[4]byte{0xca, 0xfe, 0xba, 0xbe},
		[4]byte{0xbe, 0xba, 0xfe, 0xca}:
		return true
	}
	return false
}

// MaxUnpackedSize is the most an update archive may unpack to.
var MaxUnpackedSize int64 = 8 << 30

var (
	zstdMagic = []byte{0x28, 0xB5, 0x2F, 0xFD}
	gzipMagic = []byte{0x1F, 0x8B}
)

// ExtractArchive unpacks a .tar.zst or .tar.gz update archive into dir using
// os.OpenRoot so symlinks cannot escape dir.
func ExtractArchive(r io.Reader, dir string) error {
	br := bufio.NewReader(r)
	head, err := br.Peek(4)
	if err != nil && len(head) < 2 {
		return fmt.Errorf("reading archive header: %w", err)
	}
	var trSrc io.Reader
	switch {
	case bytes.HasPrefix(head, zstdMagic):
		zr, err := zstd.NewReader(br)
		if err != nil {
			return err
		}
		defer zr.Close()
		trSrc = zr
	case bytes.HasPrefix(head, gzipMagic):
		gz, err := gzip.NewReader(br)
		if err != nil {
			return err
		}
		defer gz.Close()
		trSrc = gz
	default:
		return errors.New("unsupported update archive format")
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	tr := tar.NewReader(trSrc)
	var written int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := path.Clean(hdr.Name)
		if !fs.ValidPath(name) || name == "." {
			return fmt.Errorf("invalid path in update archive: %q", hdr.Name)
		}
		target := filepath.FromSlash(name)
		mode := fs.FileMode(hdr.Mode).Perm()
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := root.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := root.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := root.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode|0o200)
			if err != nil {
				return err
			}
			n, err := io.Copy(f, io.LimitReader(tr, MaxUnpackedSize-written+1))
			written += n
			if err == nil && written > MaxUnpackedSize {
				err = fmt.Errorf("update archive unpacks to more than %d bytes", MaxUnpackedSize)
			}
			if err == nil {
				err = f.Chmod(mode)
			}
			if cerr := f.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				return err
			}
		case tar.TypeSymlink:
			resolved := path.Join(path.Dir(name), hdr.Linkname)
			if path.IsAbs(hdr.Linkname) || !fs.ValidPath(resolved) {
				return fmt.Errorf("update archive link %q points outside the app", hdr.Name)
			}
			if err := root.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := root.Symlink(hdr.Linkname, target); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported entry in update archive: %q", hdr.Name)
		}
	}
}

// ReleaseNotes returns the section of a Markdown changelog under the
// "## <version>" or "### <version>" heading, or "## [<version>]" as in
// Keep a Changelog (a date or other text may follow the version), or "" when
// there is none. "## " headings take precedence over "### " headings so a
// "### Fixed" subsection inside "## <version>" is not treated as a version.
func ReleaseNotes(changelog, version string) string {
	lines := strings.Split(strings.ReplaceAll(changelog, "\r\n", "\n"), "\n")
	if notes := extractReleaseNotes(lines, "## ", version, false); notes != "" {
		return notes
	}
	return extractReleaseNotes(lines, "### ", version, true)
}

func extractReleaseNotes(lines []string, prefix, version string, stopAtH2 bool) string {
	var notes []string
	in := false
	version = strings.TrimPrefix(version, "v")
	for _, line := range lines {
		if stopAtH2 && in && strings.HasPrefix(line, "## ") {
			break
		}
		if rest, ok := strings.CutPrefix(line, prefix); ok {
			if in {
				break
			}
			fields := strings.Fields(rest)
			in = len(fields) > 0 && strings.TrimPrefix(headingVersion(fields[0]), "v") == version
			continue
		}
		if in {
			notes = append(notes, line)
		}
	}
	return strings.TrimSpace(strings.Join(notes, "\n"))
}

// headingVersion returns the version a changelog heading starts with.
func headingVersion(s string) string {
	if rest, ok := strings.CutPrefix(s, "["); ok {
		s, _, _ = strings.Cut(rest, "]")
	}
	return s
}

// IsPrerelease reports whether version has a SemVer prerelease suffix.
func IsPrerelease(version string) bool {
	v, _, _ := strings.Cut(strings.TrimPrefix(version, "v"), "+")
	_, pre, ok := strings.Cut(v, "-")
	return ok && pre != ""
}

// Unused slice import guard.
var _ = slices.Contains[[]string, string]
