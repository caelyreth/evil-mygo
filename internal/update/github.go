package update

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
)

// VersionPlaceholder stands for the version in tagged GitHub feeds or mirror URLs.
const VersionPlaceholder = "{version}"

// ChannelPlaceholder stands for the update channel in manifest feed URLs.
const ChannelPlaceholder = "{channel}"

// TargetPlaceholder stands for the target platform in manifest feed URLs.
const TargetPlaceholder = "{target}"

// ErrNotPublished is returned when no release is published for a feed or channel.
var ErrNotPublished = errors.New("no release is published")

// Getter gets a URL and returns the body of a successful response.
type Getter func(ctx context.Context, url string) (io.ReadCloser, error)

// GitHubAPI is the GitHub REST API, which tests replace.
var GitHubAPI = "https://api.github.com"

const (
	releasesPerPage = 100
	maxReleasePages = 10
)

// TaggedFeed returns the feed of the manifest name in the newest release of
// the GitHub repository repo ("owner/name") tagged prefix and a version.
func TaggedFeed(repo, prefix, name string) string {
	return "https://github.com/" + repo + "/releases/download/" + url.PathEscape(prefix) + VersionPlaceholder + "/" + url.PathEscape(name)
}

// ResolveChannelFeed resolves feed for target and channel:
//   - Direct .json URLs or custom URL templates ({channel}, {target}) expand inline.
//   - GitHub feeds select prereleases when channel == BetaChannel.
//   - Base HTTP/HTTPS directory feeds resolve to <feed>/<channel>/update-<target>.json.
func ResolveChannelFeed(ctx context.Context, feed, target, channel string, get Getter) (string, error) {
	if channel == "" {
		channel = DefaultChannel
	}
	if strings.Contains(feed, ChannelPlaceholder) || strings.Contains(feed, TargetPlaceholder) {
		r := strings.NewReplacer(
			ChannelPlaceholder, url.PathEscape(channel),
			TargetPlaceholder, url.PathEscape(target),
		)
		return r.Replace(feed), nil
	}
	if _, _, _, isGH := parseGitHubFeed(feed); isGH || strings.Contains(feed, VersionPlaceholder) {
		if channel == BetaChannel {
			return ResolvePrereleaseFeed(ctx, feed, get)
		}
		return ResolveFeed(ctx, feed, get)
	}
	trimmed := strings.TrimSuffix(feed, "/")
	if strings.HasSuffix(trimmed, ".json") {
		return trimmed, nil
	}
	return trimmed + "/" + url.PathEscape(channel) + "/" + ManifestName(target), nil
}

// ResolveFeed returns the URL of the manifest that feed names: feed itself,
// or for a feed of tagged GitHub releases (TaggedFeed) the manifest of the
// newest release with the tag's prefix that is neither a draft nor a prerelease.
func ResolveFeed(ctx context.Context, feed string, get Getter) (string, error) {
	if !strings.Contains(feed, VersionPlaceholder) {
		return feed, nil
	}
	repo, prefix, err := parseTaggedFeed(feed)
	if err != nil {
		return "", err
	}
	tag, err := latestTagged(ctx, get, repo, prefix)
	if err != nil {
		return "", err
	}
	return strings.Replace(feed, "/"+url.PathEscape(prefix)+VersionPlaceholder+"/", "/"+url.PathEscape(tag)+"/", 1), nil
}

func parseTaggedFeed(feed string) (repo, prefix string, err error) {
	rest, ok := strings.CutPrefix(feed, "https://github.com/")
	parts := strings.SplitN(rest, "/", 6)
	if !ok || len(parts) != 6 || parts[2] != "releases" || parts[3] != "download" || !strings.HasSuffix(parts[4], VersionPlaceholder) {
		return "", "", fmt.Errorf("%s is not a feed of GitHub releases", feed)
	}
	prefix, err = url.PathUnescape(strings.TrimSuffix(parts[4], VersionPlaceholder))
	if err != nil {
		return "", "", fmt.Errorf("%s is not a feed of GitHub releases: %w", feed, err)
	}
	return parts[0] + "/" + parts[1], prefix, nil
}

func latestTagged(ctx context.Context, get Getter, repo, prefix string) (string, error) {
	for page := 1; page <= maxReleasePages; page++ {
		u := GitHubAPI + "/repos/" + repo + "/releases?per_page=" + strconv.Itoa(releasesPerPage) + "&page=" + strconv.Itoa(page)
		releases, err := getReleases(ctx, get, u)
		if err != nil {
			return "", err
		}
		for _, r := range releases {
			if !r.Draft && !r.Prerelease && strings.HasPrefix(r.TagName, prefix) {
				return r.TagName, nil
			}
		}
		if len(releases) < releasesPerPage {
			break
		}
	}
	return "", fmt.Errorf("%w in %s tagged %s", ErrNotPublished, repo, prefix)
}

// ResolvePrereleaseFeed is ResolveFeed for apps that take prereleases too.
func ResolvePrereleaseFeed(ctx context.Context, feed string, get Getter) (string, error) {
	repo, prefix, name, ok := parseGitHubFeed(feed)
	if !ok {
		if strings.Contains(feed, VersionPlaceholder) {
			return "", fmt.Errorf("%s is not a feed of GitHub releases", feed)
		}
		return feed, nil
	}
	tag, err := highestTagged(ctx, get, repo, prefix)
	if err != nil {
		return "", err
	}
	return "https://github.com/" + repo + "/releases/download/" + url.PathEscape(tag) + "/" + url.PathEscape(name), nil
}

func parseGitHubFeed(feed string) (repo, prefix, name string, ok bool) {
	rest, ok := strings.CutPrefix(feed, "https://github.com/")
	parts := strings.SplitN(rest, "/", 6)
	if !ok || len(parts) != 6 || parts[2] != "releases" {
		return "", "", "", false
	}
	name, err := url.PathUnescape(parts[5])
	if err != nil {
		return "", "", "", false
	}
	switch {
	case parts[3] == "latest" && parts[4] == "download":
		prefix = "v"
	case parts[3] == "download" && strings.HasSuffix(parts[4], VersionPlaceholder):
		if prefix, err = url.PathUnescape(strings.TrimSuffix(parts[4], VersionPlaceholder)); err != nil {
			return "", "", "", false
		}
	default:
		return "", "", "", false
	}
	return parts[0] + "/" + parts[1], prefix, name, true
}

func highestTagged(ctx context.Context, get Getter, repo, prefix string) (string, error) {
	var best, bestVersion string
	for page := 1; page <= maxReleasePages; page++ {
		u := GitHubAPI + "/repos/" + repo + "/releases?per_page=" + strconv.Itoa(releasesPerPage) + "&page=" + strconv.Itoa(page)
		releases, err := getReleases(ctx, get, u)
		if err != nil {
			return "", err
		}
		stable := false
		for _, r := range releases {
			v, ok := strings.CutPrefix(r.TagName, prefix)
			if r.Draft || !ok || v == "" || v[0] < '0' || v[0] > '9' {
				continue
			}
			stable = stable || !r.Prerelease
			if best == "" || Compare(v, bestVersion) > 0 {
				best, bestVersion = r.TagName, v
			}
		}
		if stable || len(releases) < releasesPerPage {
			break
		}
	}
	if best == "" {
		return "", fmt.Errorf("%w in %s tagged %s", ErrNotPublished, repo, prefix)
	}
	return best, nil
}

type release struct {
	TagName    string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

func getReleases(ctx context.Context, get Getter, u string) ([]release, error) {
	body, err := get(ctx, u)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	var releases []release
	if err := json.UnmarshalRead(io.LimitReader(body, 64<<20), &releases); err != nil {
		return nil, fmt.Errorf("reading the releases of %s: %w", u, err)
	}
	return releases, nil
}
