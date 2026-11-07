// Package version records the fork release and its upstream baseline without
// linking the desktop framework into tools that only need build metadata.
package version

// Version is the independent evil-mygo release, shared by Go and npm packages.
const Version = "0.1.0"

// UpstreamVersion is the MyGo release at the last integrated upstream baseline.
const UpstreamVersion = "0.8.0"

// UpstreamRevision identifies that baseline, not the current upstream HEAD.
const UpstreamRevision = "442d09c293e9586809175fe55beff0be905459c7"

// UpstreamRepository is the original project's repository.
const UpstreamRepository = "https://github.com/egoist/mygo"

// String includes provenance for display; dependency versions use Version alone.
func String() string {
	return "evil-mygo " + Version + " (based on MyGo " + UpstreamVersion + " @ " + UpstreamRevision[:12] + ")"
}
