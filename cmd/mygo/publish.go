package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/internal/update"
)

func checkUpload(c *Config) error {
	switch u := c.Updates; {
	case u != nil && u.S3 != nil:
		if _, err := newS3Client(u.S3); err != nil {
			return err
		}
	case u == nil || u.GitHub == "":
		return fmt.Errorf("-upload needs updates.github or updates.s3 in %s", c.configName())
	}
	keys, err := c.signingKeys()
	if err != nil {
		return fmt.Errorf("updates: %w", err)
	}
	if len(keys) == 0 {
		return errors.New("-upload needs the update signing key, or installed apps could not update to this release: set MYGO_UPDATER_PRIVATE_KEY or updates.privateKey")
	}
	return nil
}

func publish(c *Config, artifacts []string) error {
	if c.Updates != nil && c.Updates.S3 != nil {
		return publishS3(c, artifacts)
	}
	return publishGitHub(c, artifacts)
}

func uploads(artifacts []string) (files, manifests []string, err error) {
	for _, a := range artifacts {
		info, err := os.Stat(a)
		if err != nil || info.IsDir() {
			continue
		}
		name := filepath.Base(a)
		switch {
		case (strings.HasPrefix(name, "update-") || strings.HasPrefix(name, "history-")) && strings.HasSuffix(name, ".json"):
			manifests = append(manifests, a)
		case name == installScriptName, name == installPS1Name, name == checksumsName:
			if !slices.ContainsFunc(files, func(f string) bool { return filepath.Base(f) == name }) {
				files = append(files, a)
			}
		case strings.HasSuffix(name, ".dmg"), strings.HasSuffix(name, ".exe") && strings.Contains(name, installerMark),
			strings.HasSuffix(name, ".deb"), strings.HasSuffix(name, ".rpm"), strings.HasSuffix(name, ".tar.gz"),
			strings.HasSuffix(name, ".tar.zst"), strings.HasSuffix(name, ".delta"):
			files = append(files, a)
		}
	}
	if len(files)+len(manifests) == 0 {
		return nil, nil, errors.New("nothing to upload")
	}
	return files, manifests, nil
}

func publishGitHub(c *Config, artifacts []string) error {
	if c.Updates == nil || c.Updates.GitHub == "" {
		return fmt.Errorf("-upload needs updates.github in %s", c.configName())
	}
	gh, err := exec.LookPath("gh")
	if err != nil {
		return errors.New("-upload needs the GitHub CLI (https://cli.github.com), signed in with gh auth login or GH_TOKEN")
	}
	repo, tag := c.Updates.GitHub, c.Updates.TagPrefix+c.Version
	files, manifests, err := uploads(artifacts)
	if err != nil {
		return err
	}
	latest := ""
	if c.Updates.tagged() {
		latest = " --latest=false"
	}
	if exec.Command(gh, "release", "view", tag, "--repo", repo).Run() == nil {
		if err := mergeReleaseChecksums(gh, repo, tag, files); err != nil {
			return err
		}
	} else {
		notes, _, _ := c.releaseNotes()
		t := con.start("Creating the draft release " + tag + " of " + repo)
		args := []string{"release", "create", tag, "--repo", repo, "--draft", "--title", tag, "--notes", notes}
		if latest != "" {
			args = append(args, "--latest=false")
		}
		if out, err := exec.Command(gh, args...).CombinedOutput(); err != nil {
			t.fail()
			return fmt.Errorf("gh release create: %v\n%s", err, out)
		}
		t.done("Created the draft release " + tag + " of " + repo)
	}
	for _, batch := range [][]string{files, manifests} {
		if len(batch) == 0 {
			continue
		}
		t := con.start("Uploading " + uploadNames(batch))
		args := append([]string{"release", "upload", tag, "--repo", repo, "--clobber"}, batch...)
		if out, err := exec.Command(gh, args...).CombinedOutput(); err != nil {
			t.fail()
			return fmt.Errorf("gh release upload: %v\n%s", err, out)
		}
		t.done("Uploaded " + uploadNames(batch))
	}
	if v, _, _ := strings.Cut(c.Version, "+"); strings.Contains(v, "-") {
		latest += " --prerelease"
	}
	logf("Uploaded to the release %s of %s; publish it when every platform is there:\n  gh release edit %s --repo %s --draft=false%s", tag, repo, tag, repo, latest)
	return nil
}

func mergeReleaseChecksums(gh, repo, tag string, files []string) error {
	i := slices.IndexFunc(files, func(f string) bool { return filepath.Base(f) == checksumsName })
	if i < 0 {
		return nil
	}
	dir, err := os.MkdirTemp("", "mygo-checksums-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	out, err := exec.Command(gh, "release", "view", tag, "--repo", repo, "--json", "assets", "--jq", ".assets[].name").Output()
	if err != nil {
		return fmt.Errorf("gh release view %s: %v", tag, err)
	}
	if !slices.Contains(strings.Split(string(out), "\n"), checksumsName) {
		return nil
	}
	if out, err := exec.Command(gh, "release", "download", tag, "--repo", repo, "--pattern", checksumsName, "--dir", dir).CombinedOutput(); err != nil {
		return fmt.Errorf("gh release download %s %s: %v\n%s", tag, checksumsName, err, out)
	}
	earlier, err := os.ReadFile(filepath.Join(dir, checksumsName))
	if err != nil {
		return err
	}
	return mergeChecksums(files[i], earlier, "")
}

// publishS3 uploads binaries to updates.s3 and, unless splitHosting is
// configured (manifest hosted on a dedicated server), uploads channel-scoped
// manifests last with Cache-Control: no-cache.
func publishS3(c *Config, artifacts []string) error {
	bucket, err := newS3Client(c.Updates.S3)
	if err != nil {
		return err
	}
	files, manifests, err := uploads(artifacts)
	if err != nil {
		return err
	}
	if err := mergeS3Checksums(c, files); err != nil {
		return err
	}
	channel := c.Updates.effectiveChannel(c.Version)
	var histories, clientManifests []string
	for _, m := range manifests {
		if strings.HasPrefix(filepath.Base(m), "history-") {
			histories = append(histories, m)
		} else {
			clientManifests = append(clientManifests, m)
		}
	}
	split := c.Updates.splitHosting()

	for _, f := range files {
		name := filepath.Base(f)
		t := con.start("Uploading " + name)
		t.set(sizeOf(f))
		if err := t.end(bucket.put(bucket.key(name), f, uploadHeader(name)), "Uploaded "+name); err != nil {
			return err
		}
	}
	for _, h := range histories {
		name := filepath.Base(h)
		key := bucket.key(channel + "/" + name)
		t := con.start("Uploading " + channel + "/" + name)
		t.set(sizeOf(h))
		if err := t.end(bucket.put(key, h, uploadHeader(name)), "Uploaded "+channel+"/"+name); err != nil {
			return err
		}
	}
	if !split {
		for _, m := range clientManifests {
			name := filepath.Base(m)
			key := bucket.key(channel + "/" + name)
			t := con.start("Uploading " + channel + "/" + name)
			t.set(sizeOf(m))
			if err := t.end(bucket.put(key, m, uploadHeader(name)), "Uploaded "+channel+"/"+name); err != nil {
				return err
			}
		}
		logf("Uploaded to s3://%s/%s, which %s serves", bucket.bucket, bucket.prefix, c.Updates.URL)
		return nil
	}
	logf("Uploaded artifacts to s3://%s/%s (%s); deploy manifests to %s/%s/:", bucket.bucket, bucket.prefix, c.Updates.URL, strings.TrimSuffix(c.Updates.FeedURL, "/"), channel)
	for _, m := range clientManifests {
		logf("  %s", m)
	}
	return nil
}

func mergeS3Checksums(c *Config, files []string) error {
	i := slices.IndexFunc(files, func(f string) bool { return filepath.Base(f) == checksumsName })
	if i < 0 {
		return nil
	}
	u := strings.TrimSuffix(c.Updates.URL, "/") + "/" + checksumsName
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "mygo/"+version)
	req.Header.Set("Cache-Control", "no-cache")
	resp, err := (&http.Client{Timeout: time.Minute}).Do(req)
	if err != nil {
		return fmt.Errorf("reading the published %s: %w", checksumsName, err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound, http.StatusForbidden:
		return nil
	default:
		return fmt.Errorf("reading the published %s: GET %s: %s", checksumsName, u, resp.Status)
	}
	earlier, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return fmt.Errorf("reading the published %s: %w", checksumsName, err)
	}
	return mergeChecksums(files[i], earlier, c.Version)
}

func uploadHeader(name string) http.Header {
	typ, ok := contentTypes[filepath.Ext(name)]
	if !ok {
		typ = "application/octet-stream"
	}
	h := http.Header{"Content-Type": {typ}}
	if name == installScriptName || name == installPS1Name || name == checksumsName ||
		((strings.HasPrefix(name, "update-") || strings.HasPrefix(name, "history-")) && strings.HasSuffix(name, ".json")) {
		h.Set("Cache-Control", "no-cache")
	} else {
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	return h
}

var contentTypes = map[string]string{
	".dmg":  "application/x-apple-diskimage",
	".exe":  "application/vnd.microsoft.portable-executable",
	".deb":  "application/vnd.debian.binary-package",
	".rpm":  "application/x-rpm",
	".gz":   "application/gzip",
	".zst":  "application/zstd",
	".json": "application/json",
	".sh":   "text/plain; charset=utf-8",
	".ps1":  "text/plain; charset=utf-8",
}

func uploadNames(files []string) string {
	if len(files) == 1 {
		return filepath.Base(files[0])
	}
	return fmt.Sprintf("%d files", len(files))
}

var _ = update.ManifestName
