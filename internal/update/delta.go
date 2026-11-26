package update

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
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

// Delta updates v2 ("mygo delta 2\n"):
//
//	deltaMagic, the length of the index (uvarint), the index (JSON, zstd),
//	and the data of the files, in index order: a zstd-compressed bsdiff patch
//	(against e.From) or the new file compressed with zstd.
//
// Blobs are streamed into a temporary file while building so large updates do
// not retain all patches and compressed files in heap memory at once.
const deltaMagic = "mygo delta 2\n"

type deltaIndex struct {
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
	Data   int64  `json:"data,omitzero"`
}

func (e *deltaEntry) isFile() bool { return !e.Dir && e.Link == "" }

const maxDiff = 256 << 20

const stapledTicket = "Contents/CodeResources"

// WriteDelta writes the delta that turns the app in oldDir (version from)
// into the app in newDir (version to): entries of newDir, or all of it when
// entries is nil.
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

	chunksFile, err := os.CreateTemp("", "mygo-delta-chunks-*")
	if err != nil {
		return err
	}
	chunksPath := chunksFile.Name()
	defer func() {
		chunksFile.Close()
		os.Remove(chunksPath)
	}()

	index := deltaIndex{From: from, Version: to}
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
					blob := zstdCompress(b)
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
						if patch := Diff(prev, b); len(patch) < len(blob) {
							e.From, blob = baseFrom, patch
						}
					}
					e.Data = int64(len(blob))
					if _, err := chunksFile.Write(blob); err != nil {
						return err
					}
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
	js = zstdCompress(js)
	head := binary.AppendUvarint([]byte(deltaMagic), uint64(len(js)))
	if _, err := w.Write(append(head, js...)); err != nil {
		return err
	}
	if _, err := chunksFile.Seek(0, io.SeekStart); err != nil {
		return err
	}
	_, err = io.Copy(w, chunksFile)
	return err
}

func relPath(dir, p string) string {
	rel, _ := filepath.Rel(dir, p)
	return filepath.ToSlash(rel)
}

const maxDeltaIndex = 16 << 20

var errDamagedDelta = errors.New("damaged delta update")

// ApplyDelta applies a delta update of size bytes read from r against oldDir
// to create newDir, verifying every output file's size and SHA-256.
func ApplyDelta(r io.ReaderAt, size int64, from, to, oldDir, newDir string) error {
	var head [len(deltaMagic) + binary.MaxVarintLen64]byte
	n, _ := r.ReadAt(head[:min(size, int64(len(head)))], 0)
	if !bytes.HasPrefix(head[:n], []byte(deltaMagic)) {
		return errors.New("not a delta update")
	}
	var indexLen uint64
	k, ok := uvarints(head[len(deltaMagic):n], &indexLen)
	start := int64(len(deltaMagic)) + k
	if !ok || indexLen > uint64(size-start) {
		return errDamagedDelta
	}
	dec, err := zstd.NewReader(io.NewSectionReader(r, start, int64(indexLen)))
	if err != nil {
		return errDamagedDelta
	}
	js, err := io.ReadAll(io.LimitReader(dec, maxDeltaIndex+1))
	dec.Close()
	if err != nil || len(js) > maxDeltaIndex {
		return errDamagedDelta
	}
	var index deltaIndex
	if err := json.Unmarshal(js, &index); err != nil {
		return errDamagedDelta
	}
	if index.From != from || index.Version != to {
		return fmt.Errorf("the delta updates %s to %s, not %s to %s", index.From, index.Version, from, to)
	}

	offset := start + int64(indexLen)
	seen := map[string]bool{}
	for _, e := range index.Entries {
		if !fs.ValidPath(e.Path) || e.Path == "." || seen[e.Path] || e.Size < 0 || e.Data < 0 || e.Data > size-offset ||
			e.Dir && e.Link != "" || !e.isFile() && (e.Size != 0 || e.Data != 0 || e.From != "") ||
			e.From != "" && !fs.ValidPath(e.From) || e.isFile() && e.From == "" && e.Data == 0 {
			return errDamagedDelta
		}
		if e.Link != "" {
			if target := path.Join(path.Dir(e.Path), e.Link); path.IsAbs(e.Link) || !fs.ValidPath(target) {
				return fmt.Errorf("delta update link %q points outside the app", e.Path)
			}
		}
		seen[e.Path] = true
		offset += e.Data
	}
	if offset != size {
		return errDamagedDelta
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
	offset = start + int64(indexLen)
	for _, e := range index.Entries {
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
			err = applyFile(e, io.NewSectionReader(r, offset, e.Data), oldRoot, newRoot)
			offset += e.Data
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func applyFile(e deltaEntry, data *io.SectionReader, oldRoot, newRoot *os.Root) error {
	f, err := newRoot.OpenFile(filepath.FromSlash(e.Path), os.O_CREATE|os.O_EXCL|os.O_WRONLY, fs.FileMode(e.Mode).Perm()|0o200)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	out := &limitWriter{w: io.MultiWriter(f, h), n: e.Size}
	if e.From == "" {
		zr, zerr := zstd.NewReader(data)
		if zerr != nil {
			return errDamagedDelta
		}
		_, err = io.Copy(out, zr)
		if err == nil && !streamEOF(zr) {
			err = errDamagedPatch
		}
		zr.Close()
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
		case e.Data == 0:
			_, err = io.Copy(out, src)
		default:
			err = Patch(out, e.Size, src, info.Size(), data, e.Data)
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
