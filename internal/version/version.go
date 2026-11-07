// Package version records the fork release and its upstream baseline without
// linking the desktop framework into tools that only need build metadata.
package version

// Version is the independent evil-mygo release, shared by Go and npm packages.
const Version = "0.1.1"

// UpstreamVersion is the MyGo release at the last integrated upstream baseline.
const UpstreamVersion = "0.8.3"

// UpstreamRevision identifies that baseline, not the current upstream HEAD.
const UpstreamRevision = "94f22c384d0ecd8486899e5a4662cbec2577d310"

// UpstreamRepository is the original project's repository.
const UpstreamRepository = "https://github.com/egoist/mygo"

// String includes provenance for display; dependency versions use Version alone.
func String() string {
	return "evil-mygo " + Version + " (based on MyGo " + UpstreamVersion + " @ " + UpstreamRevision[:12] + ")"
}
