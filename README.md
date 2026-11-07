# evil-mygo

A separately maintained fork of [egoist/mygo](https://github.com/egoist/mygo),
focused on native GPU scenes and interactive UI on three-dimensional surfaces.
See the [framework documentation](docs/README.md) for the shared MyGo APIs.

Changes from upstream:

- macOS native UI uses WebGPU through wgpu-native over Metal; the direct Metal
  renderer is retired.
- Custom WebGPU canvases share GPU textures with native `ui.UILayer` content.
  Surface mapping routes pointer input, focus, IME and accessibility onto the
  displayed geometry without a CPU pixel round trip.
- Extended color, imported-texture synchronization, partial presentation and
  deferred first-frame handling support these GPU surfaces.
- [Silk flag](examples/layer-membrane) and
  [material archive](examples/rhine-scene) examples combine procedural geometry
  with live native UI. The archive needs no external model files.

Use `github.com/caelyreth/evil-mygo` for Go imports. The package is still `mygo`
and the CLI is still `mygo`. npm packages use the `@caelyreth/evil-mygo-` prefix,
including `@caelyreth/evil-mygo-runtime` and `@caelyreth/evil-mygo-cli`.

Current release: **0.1.1**, based on **MyGo 0.8.3** at `94f22c384d0e`.
See the [changelog](CHANGELOG.md) for the upstream sync. Fork releases have
independent SemVer versions.
`mygo.Version` identifies the fork; `mygo.UpstreamVersion` and
`mygo.UpstreamRevision` record the integrated upstream baseline.
`mygo version` prints both. Release and provenance commands are in the
[architecture guide](docs/architecture.md#releasing).

[MIT](LICENSE). Original MyGo attribution and third-party notices are retained.
