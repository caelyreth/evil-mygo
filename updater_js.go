//go:build js

package mygo

import (
	"context"
	"errors"
	"io"
	"time"
)

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

// UpdaterModule installs new versions of the app. In browsers updates are disabled.
type UpdaterModule struct{}

// Updater installs new versions of the app.
var Updater = &UpdaterModule{}

var (
	// ErrUpdatesDisabled is returned by Check for builds without updates.
	ErrUpdatesDisabled = errors.New("mygo: updates are not enabled for this build")
	// ErrManualUpdateRequired is returned when a manual reinstall is required.
	ErrManualUpdateRequired = errors.New("mygo: this update requires a manual reinstall")
)

// ManifestQuery describes a manifest fetch request.
type ManifestQuery struct {
	Target, Channel, Current, Locale, ETag string
}

// ManifestResult is returned by UpdateSource.Manifest.
type ManifestResult struct {
	Body        io.ReadCloser
	ETag        string
	NotModified bool
}

// CheckOptions configures an update check via CheckWith.
type CheckOptions struct {
	Channel, Locale string
}

// Update is a newer version of the app, found by Updater.Check.
type Update struct {
	Version              string
	Channel              string
	Notes                string
	Date                 time.Time
	RequiresManualUpdate bool
	Sources              []string
}

// NotesFor returns Notes.
func (up *Update) NotesFor(string) string {
	if up == nil {
		return ""
	}
	return up.Notes
}

// UpdateSource supplies update manifests and asset streams. Browsers do not self-update.
type UpdateSource interface {
	Manifest(ctx context.Context, q ManifestQuery) (ManifestResult, error)
	Open(ctx context.Context, url string, offset int64) (io.ReadCloser, bool, error)
}

func (u *UpdaterModule) Availability() UpdateAvailability { return UpdateUnavailableUnconfigured }
func (u *UpdaterModule) Enabled() bool                    { return false }
func (u *UpdaterModule) Target() string                   { return "js-wasm" }
func (u *UpdaterModule) DefaultChannel() string           { return "stable" }
func (u *UpdaterModule) Channel() string                  { return "stable" }
func (u *UpdaterModule) SetChannel(string)                {}
func (u *UpdaterModule) SetSource(UpdateSource)           {}
func (u *UpdaterModule) SetPrereleases(bool)              {}
func (u *UpdaterModule) Prereleases() bool                { return false }

func (u *UpdaterModule) Check(ctx context.Context) (*Update, error) {
	return nil, ErrUpdatesDisabled
}

func (u *UpdaterModule) CheckWith(ctx context.Context, opts CheckOptions) (*Update, error) {
	return nil, ErrUpdatesDisabled
}

func (up *Update) Install(ctx context.Context, progress func(downloaded, total int64)) error {
	return ErrUpdatesDisabled
}

func relaunchNow() {}

func cleanUpdateLeftovers() {}
