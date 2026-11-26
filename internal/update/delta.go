package update

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"github.com/klauspost/compress/zstd"
)

// Delta updates are standard .tar.zst archives containing:
//   - index.json: UTF-8 JSON index describing entries, SHA-256 digests, and
//     blob references ("format": "app-delta-tar-v1").
//   - blobs/<n>.vcdiff: pure IETF RFC 3284 VCDIFF binary patches.
//   - blobs/<n>: raw full replacement files (compressed by the outer zstd stream).
//
// Blobs are staged to a temporary directory before streaming index.json and
// members through a single Zstandard-compressed tar writer.
const (
	deltaFormat    = "app-delta-tar-v1"
	deltaIndexName = "index.json"
	maxDiff        = 256 << 20
	maxDeltaIndex  = 16 << 20
	stapledTicket  = "Contents/CodeResources"
)

type deltaIndex struct {
	Format  string       `json:"format"`
	From    string       `json:"from"`
	Version string       `json:"version"`
	Entries []deltaEntry `json:"entries"`
}

type deltaEntry struct {
	Path   string `json:"path"`
	Dir    bool   `json:"dir,omitzero"`
	Link   string `json:"link,omitzero"`
	Mode   uint32 `json:"mode,omitzero"`
	Size   int64  `json:"size,omitzero"`
	SHA256 string `json:"sha256,omitzero"`
	From   string `json:"from,omitzero"`
	Blob   string `json:"blob,omitzero"`
}

func (e *deltaEntry) isFile() bool { return !e.Dir && e.Link == "" }

// WriteDelta writes a standard .tar.zst delta containing index.json, raw files,
// and IETF RFC 3284 .vcdiff patches that turns oldDir (version from) into
// newDir (version to).
func WriteDelta(w io.Writer, from, to, oldDir, newDir string, entries []string) error {
	type oldFile struct {
		sum  string
		size int64
	}
	old := map[string]oldFile{}
	bySum := map[string]string{}
	byBaseCount := map[string]int{}
	byBasePath := map[string]string{}

	err := filepath.WalkDir(oldDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		rel := relPath(oldDir, p)
		if rel == stapledTicket || rel == FileList {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		h := sha256.New()
		size, err := io.Copy(h, f)
		if err != nil {
			return err
		}
		sum := hex.EncodeToString(h.Sum(nil))
		old[rel] = oldFile{sum, size}
		if _, ok := bySum[sum]; !ok {
			bySum[sum] = rel
		}
		base := path.Base(rel)
		byBaseCount[base]++
		if byBaseCount[base] == 1 {
			byBasePath[base] = rel
		} else {
			delete(byBasePath, base)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if entries == nil {
		all, err := os.ReadDir(newDir)
		if err != nil {
			return err
		}
		for _, e := range all {
			entries = append(entries, e.Name())
		}
	}

	blobWork, err := os.MkdirTemp("", "app-delta-blobs-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(blobWork)

	type stagedBlob struct {
		name string
		path string
		size int64
	}
	var blobs []stagedBlob

	index := deltaIndex{Format: deltaFormat, From: from, Version: to}
	for _, top := range entries {
		err := filepath.WalkDir(filepath.Join(newDir, top), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			e := deltaEntry{Path: relPath(newDir, p)}
			switch {
			case d.IsDir():
				e.Dir = true
			case d.Type()&fs.ModeSymlink != 0:
				if e.Link, err = os.Readlink(p); err != nil {
					return err
				}
				e.Link = filepath.ToSlash(e.Link)
			case d.Type().IsRegular():
				info, err := d.Info()
				if err != nil {
					return err
				}
				b, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				sum := sha256.Sum256(b)
				e.Mode, e.Size, e.SHA256 = uint32(info.Mode().Perm()), int64(len(b)), hex.EncodeToString(sum[:])
				o := old[e.Path]
				if o.sum == e.SHA256 {
					e.From = e.Path
				} else if src, ok := bySum[e.SHA256]; ok {
					e.From = src
				} else {
					blobData := b
					blobName := fmt.Sprintf("blobs/%05d", len(blobs)+1)
					baseFrom, baseFile := "", oldFile{}
					if o.size > 0 {
						baseFrom, baseFile = e.Path, o
					} else if moved, ok := byBasePath[path.Base(e.Path)]; ok {
						baseFrom, baseFile = moved, old[moved]
					}
					if baseFrom != "" && baseFile.size > 0 && baseFile.size <= maxDiff && len(b) <= maxDiff {
						prev, err := os.ReadFile(filepath.Join(oldDir, filepath.FromSlash(baseFrom)))
						if err != nil {
							return err
						}
						if patch := EncodeVCDIFF(prev, b); len(patch) < len(b) {
							e.From = baseFrom
							blobData = patch
							blobName = fmt.Sprintf("blobs/%05d.vcdiff", len(blobs)+1)
						}
					}
					e.Blob = blobName
					bp := filepath.Join(blobWork, fmt.Sprintf("blob-%05d", len(blobs)+1))
					if err := os.WriteFile(bp, blobData, 0o600); err != nil {
						return err
					}
					blobs = append(blobs, stagedBlob{name: blobName, path: bp, size: int64(len(blobData))})
				}
			default:
				return fmt.Errorf("%s is not a file, directory or link", p)
			}
			index.Entries = append(index.Entries, e)
			return nil
		})
		if err != nil {
			return err
		}
	}

	js, err := json.Marshal(index)
	if err != nil {
		return err
	}
	js = append(js, '\n')

	zw, err := zstd.NewWriter(w, zstd.WithEncoderLevel(zstd.SpeedBestCompression))
	if err != nil {
		return err
	}
	tw := tar.NewWriter(zw)
	if err := tw.WriteHeader(&tar.Header{
		Name: deltaIndexName,
		Mode: 0o644,
		Size: int64(len(js)),
	}); err != nil {
		_ = tw.Close()
		_ = zw.Close()
		return err
	}
	if _, err := tw.Write(js); err != nil {
		_ = tw.Close()
		_ = zw.Close()
		return err
	}
	for _, b := range blobs {
		if err := tw.WriteHeader(&tar.Header{
			Name: b.name,
			Mode: 0o644,
			Size: b.size,
		}); err != nil {
			_ = tw.Close()
			_ = zw.Close()
			return err
		}
		f, err := os.Open(b.path)
		if err != nil {
			_ = tw.Close()
			_ = zw.Close()
			return err
		}
		_, cerr := io.Copy(tw, f)
		f.Close()
		if cerr != nil {
			_ = tw.Close()
			_ = zw.Close()
			return cerr
		}
	}
	if err := tw.Close(); err != nil {
		_ = zw.Close()
		return err
	}
	return zw.Close()
}

func relPath(dir, p string) string {
	rel, _ := filepath.Rel(dir, p)
	return filepath.ToSlash(rel)
}

var errDamagedDelta = errors.New("damaged delta update")

// ApplyDelta applies a standard .tar.zst delta update (index.json + raw files
// and RFC 3284 .vcdiff patches) of size bytes read from r against oldDir to
// create newDir, verifying every output file's size and SHA-256.
func ApplyDelta(r io.Reader, size int64, from, to, oldDir, newDir string) error {
	if size <= 0 {
		return errDamagedDelta
	}
	lr := &io.LimitedReader{R: r, N: size}
	zr, err := zstd.NewReader(lr)
	if err != nil {
		return errDamagedDelta
	}
	defer zr.Close()
	tr := tar.NewReader(zr)

	hdr, err := tr.Next()
	if err != nil || hdr.Name != deltaIndexName || (hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeRegA) || hdr.Size <= 0 || hdr.Size > maxDeltaIndex {
		return errDamagedDelta
	}
	js, err := io.ReadAll(io.LimitReader(tr, maxDeltaIndex+1))
	if err != nil || len(js) != int(hdr.Size) || len(js) > maxDeltaIndex {
		return errDamagedDelta
	}
	var index deltaIndex
	if err := json.Unmarshal(js, &index); err != nil || index.Format != deltaFormat {
		return errDamagedDelta
	}
	if index.From != from || index.Version != to {
		return fmt.Errorf("the delta updates %s to %s, not %s to %s", index.From, index.Version, from, to)
	}

	seen := map[string]bool{}
	expectedBlobs := map[string]*deltaEntry{}
	for i := range index.Entries {
		e := &index.Entries[i]
		if !fs.ValidPath(e.Path) || e.Path == "." || seen[e.Path] || e.Size < 0 ||
			e.Dir && e.Link != "" || !e.isFile() && (e.Size != 0 || e.Blob != "" || e.From != "") ||
			e.From != "" && !fs.ValidPath(e.From) || e.isFile() && e.From == "" && e.Blob == "" {
			return errDamagedDelta
		}
		if e.Link != "" {
			if target := path.Join(path.Dir(e.Path), e.Link); path.IsAbs(e.Link) || !fs.ValidPath(target) {
				return fmt.Errorf("delta update link %q points outside the app", e.Path)
			}
		}
		seen[e.Path] = true
		if e.Blob != "" {
			if !fs.ValidPath(e.Blob) || expectedBlobs[e.Blob] != nil {
				return errDamagedDelta
			}
			expectedBlobs[e.Blob] = e
		}
	}

	if err := os.Mkdir(newDir, 0o755); err != nil {
		return err
	}
	oldRoot, err := os.OpenRoot(oldDir)
	if err != nil {
		return err
	}
	defer oldRoot.Close()
	newRoot, err := os.OpenRoot(newDir)
	if err != nil {
		return err
	}
	defer newRoot.Close()

	// Create directories, symlinks, and direct-copy entries first.
	for _, e := range index.Entries {
		if e.Blob != "" {
			continue
		}
		name := filepath.FromSlash(e.Path)
		if dir := filepath.Dir(name); dir != "." {
			if err := newRoot.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}
		switch {
		case e.Dir:
			err = newRoot.MkdirAll(name, 0o755)
		case e.Link != "":
			err = newRoot.Symlink(filepath.FromSlash(e.Link), name)
		default:
			err = applyFile(e, nil, oldRoot, newRoot)
		}
		if err != nil {
			return err
		}
	}

	// Stream each tar blob into its target file.
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errDamagedDelta
		}
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeRegA {
			return errDamagedDelta
		}
		e, ok := expectedBlobs[hdr.Name]
		if !ok {
			return errDamagedDelta
		}
		delete(expectedBlobs, hdr.Name)

		name := filepath.FromSlash(e.Path)
		if dir := filepath.Dir(name); dir != "." {
			if err := newRoot.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}
		if err := applyFile(*e, tr, oldRoot, newRoot); err != nil {
			return err
		}
	}
	if len(expectedBlobs) != 0 || !streamEOF(zr) || lr.N != 0 {
		return errDamagedDelta
	}
	return nil
}

func applyFile(e deltaEntry, blob io.Reader, oldRoot, newRoot *os.Root) error {
	f, err := newRoot.OpenFile(filepath.FromSlash(e.Path), os.O_CREATE|os.O_EXCL|os.O_WRONLY, fs.FileMode(e.Mode).Perm()|0o200)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	out := &limitWriter{w: io.MultiWriter(f, h), n: e.Size}
	if e.From == "" {
		_, err = io.Copy(out, blob)
	} else {
		var src *os.File
		if src, err = oldRoot.Open(filepath.FromSlash(e.From)); err != nil {
			return err
		}
		defer src.Close()
		var info fs.FileInfo
		switch info, err = src.Stat(); {
		case err != nil:
		case !info.Mode().IsRegular():
			err = fmt.Errorf("%s is not a file", e.From)
		case e.Blob == "":
			_, err = io.Copy(out, src)
		default:
			err = DecodeVCDIFF(out, e.Size, src, info.Size(), blob)
		}
	}
	if err == nil {
		err = f.Close()
	}
	if err == errDamagedPatch || err == errTooLong || err == nil && (out.n != 0 || hex.EncodeToString(h.Sum(nil)) != e.SHA256) {
		return fmt.Errorf("the delta update made %s wrong", e.Path)
	}
	if err == nil {
		err = newRoot.Chmod(filepath.FromSlash(e.Path), fs.FileMode(e.Mode).Perm())
	}
	return err
}

var errTooLong = errors.New("too long")

type limitWriter struct {
	w io.Writer
	n int64
}

func (l *limitWriter) Write(b []byte) (int, error) {
	if int64(len(b)) > l.n {
		return 0, errTooLong
	}
	l.n -= int64(len(b))
	return l.w.Write(b)
}
