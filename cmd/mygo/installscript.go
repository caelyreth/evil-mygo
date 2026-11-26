package main

import (
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"unicode"

	"github.com/egoist/mygo/internal/update"
)

// The install script, install.sh, installs the app for the user on Linux,
// without root or a package manager:
//
//	curl -fsSL https://github.com/me/my-app/releases/latest/download/install.sh | sh
//
// On Linux it installs the archive of the app in ~/.local/<name>.app, where
// the app can update itself, with the commands of linux.command in
// ~/.local/bin (never over a file it did not make), and
// registers its desktop entry and file types (writeLinuxDesktop) with the
// paths of the install, as updates do again (update.RefreshDesktopEntry).
// It warns when WebKitGTK is missing, with the command that installs it,
// unless the app shows no web page (linux.webview false). It installs the
// archive it is given, else the one of its version next to it, else, with
// updates, the latest version, which the manifest of the machine's target
// names, after checking its size and its Ed25519 signature against
// updates.publicKey, as the app's updater does, with openssl pkeyutl
// (OpenSSL 3.0 or later). Without such an openssl it downloads nothing and
// says so: an archive given to it is not checked.
//
// For tests, <NAME>_DOWNLOAD_URL (the slug of the app in capitals, such as
// MY_APP_DOWNLOAD_URL) is where the manifests are instead.

const installScriptName = "install.sh"

// writeInstallScript writes install.sh into dir and returns its path.
func writeInstallScript(c *Config, dir string) (string, error) {
	path := filepath.Join(dir, installScriptName)
	return path, os.WriteFile(path, []byte(installScript(c)), 0o755)
}

func installScript(c *Config) string {
	data := map[string]string{
		"Name":       c.Name,
		"Title":      strings.Join(strings.Fields(c.Name), " "),
		"Slug":       c.slug(),
		"Commands":   strings.Join(c.Linux.Command, " "),
		"Version":    c.Version,
		"Bundle":     c.executableName() + ".app",
		"Identifier": c.Identifier,
		"Env":        envName(c.slug()),
		"Channel":    "",
		"Releases":   "",
		"API":        "",
		"TagPrefix":  "",
		"PublicKey":  "",
		"Updates":    "",
	}
	switch u := c.Updates; {
	case u != nil && u.tagged():
		data["Releases"] = "https://github.com/" + u.GitHub + "/releases/download/"
		data["API"] = update.GitHubAPI + "/repos/" + u.GitHub + "/releases?per_page=100"
		data["TagPrefix"] = u.TagPrefix
		data["Curl"] = "curl -fsSL " + c.updateFile(installScriptName, false) + " | sh"
	case u != nil:
		data["Releases"] = c.updateFile("", true)
		data["Curl"] = "curl -fsSL " + c.updateFile(installScriptName, true) + " | sh"
	}
	if c.Updates != nil {
		data["Updates"] = "yes"
		data["Channel"] = c.Updates.effectiveChannel(c.Version)
		ring, _, err := c.Updates.effectiveKeyring()
		if err == nil && len(ring) > 0 {
			data["PublicKey"] = strings.TrimSpace(c.Updates.PublicKey)
			if data["PublicKey"] == "" && len(c.Updates.PublicKeys) > 0 {
				data["PublicKey"] = strings.TrimSpace(c.Updates.PublicKeys[0])
			}
		}
	}
	var b strings.Builder
	if err := installScriptTemplate.Execute(&b, data); err != nil {
		panic(err)
	}
	return b.String()
}

// commandsPhrase names the commands of linux.command, separated by spaces
// in list, as the opening comment of install.sh does: "the my-app command",
// "the my-app and ma commands".
func commandsPhrase(list string) string {
	names := strings.Fields(list)
	switch n := len(names); n {
	case 0:
		return ""
	case 1:
		return "the " + names[0] + " command"
	default:
		return "the " + strings.Join(names[:n-1], ", ") + " and " + names[n-1] + " commands"
	}
}

// envName is the prefix of the environment variables of an install script
// of the app slug: MY_APP for my-app.
func envName(slug string) string {
	name := strings.Map(func(r rune) rune {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			return unicode.ToUpper(r)
		}
		return '_'
	}, slug)
	if name == "" || name[0] >= '0' && name[0] <= '9' {
		name = "APP_" + name
	}
	return name
}

// shQuote quotes s for a POSIX shell.
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

var installScriptTemplate = template.Must(template.New("").Funcs(template.FuncMap{"q": shQuote, "phrase": commandsPhrase}).Parse(`#!/bin/sh
# Installs {{.Title}} for the current user on Linux, without root: the app in
# ~/.local/{{.Slug}}.app{{with .Commands}}, {{phrase .}} in ~/.local/bin,{{end}} and its entry
# in the applications menu.
# Made by mygo build.
#
{{- with .Curl}}
#   {{.}}
{{- end}}
#   sh install.sh [ARCHIVE]
#   sh install.sh --uninstall
set -eu

app_name={{q .Name}}
name={{q .Slug}}
# The commands in ~/.local/bin that run the app (linux.command), if any,
# separated by spaces.
command_names={{q .Commands}}
version={{q .Version}}
# Where the manifests of the latest version are, with updates: in the
# release of the newest tag with the prefix when the repository's releases
# (api) hold others too.
releases={{q .Releases}}
{{- if .Updates}}
api={{q .API}}
tag_prefix={{q .TagPrefix}}
channel={{q .Channel}}
public_key={{q .PublicKey}}
# For tests only: where the manifests are instead.
download_url="{{"${"}}{{.Env}}_DOWNLOAD_URL:-}"
{{- end}}

usage() {
	cat <<EOF
Usage: sh install.sh [ARCHIVE | --uninstall]

Installs $app_name for you on Linux, in ~/.local: ARCHIVE, else
$name-$version-<platform>.tar.gz next to this script{{if .Curl}}, else the latest
version, which it downloads{{end}}.
--uninstall removes it, and leaves its settings and data in place.
EOF
}

fail() {
	echo "install.sh: $*" >&2
	exit 1
}

fetch() {
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL "$1"
	else
		wget -qO- "$1"
	fi
}

main() {
	if [ $# -gt 1 ]; then
		usage >&2
		exit 2
	fi
	case "${1:-}" in
	-h | --help)
		usage
		return
		;;
	--uninstall) ;;
	-*)
		usage >&2
		exit 2
		;;
	esac
{{- if .Updates}}
	if [ -n "$download_url" ]; then
		releases="${download_url%/}/"
		api=
	fi
{{- end}}
	linux "$@"
}

linux() {
	app_dir="$HOME/.local/$name.app"
	bin_dir="$HOME/.local/bin"
	data_home="${XDG_DATA_HOME:-$HOME/.local/share}"
	desktop_file="$data_home/applications/$name.desktop"
	mime_file="$data_home/mime/packages/$name.xml"
	if [ "${1:-}" = --uninstall ]; then
		uninstall
		return
	fi

	[ "$(uname -s)" = Linux ] || fail "$app_name installs on Linux"
	[ "$(id -u)" != 0 ] || fail "run this as the user to install $app_name for, not as root"
	# The desktop entry holds the path, which these characters would break.
	[ "$(printf '%s' "$app_dir" | tr -d '"$%\\\140\n')" = "$app_dir" ] ||
		fail "cannot install in $app_dir: the path has characters a desktop entry cannot hold"
	case "$(uname -m)" in
	x86_64 | amd64) target=linux-amd64 ;;
	aarch64 | arm64) target=linux-arm64 ;;
	*) fail "$app_name is not made for $(uname -m)" ;;
	esac

	work="$(mktemp -d)"
	staging="$HOME/.local/.$name.app.install"
	trap 'rm -rf "$work" "$staging"' EXIT
	trap 'exit 1' HUP INT TERM

	archive="${1:-}"
	if [ -z "$archive" ] && [ -f "$0" ] && [ -f "$(dirname -- "$0")/$name-$version-$target.tar.gz" ]; then
		archive="$(dirname -- "$0")/$name-$version-$target.tar.gz"
	fi
	if [ -z "$archive" ]; then
		[ -n "$releases" ] || fail "no $name-$version-$target.tar.gz next to this script: pass the archive to install"
{{- if .Updates}}
		command -v curl >/dev/null 2>&1 || command -v wget >/dev/null 2>&1 || fail "downloading $app_name needs curl or wget"
		if [ -n "$api" ]; then
			tag="$(latest_tag)" || fail "could not list the releases at $api"
			[ -n "$tag" ] || fail "no release of $app_name is published"
			manifest="$releases$tag/update-$target.json"
		elif [ -n "$download_url" ]; then
			manifest="${releases}update-$target.json"
		else
			manifest="${releases}${channel}/update-$target.json"
		fi
		# Fail before downloading when nothing here checks Ed25519.
		has_ed25519 || fail "checking the signature of $app_name needs OpenSSL 3.0 or later (the openssl command): install it, or download $name-<version>-$target.tar.gz and run sh install.sh with it"
		fetch "$manifest" >"$work/update.json" || fail "could not download $manifest"
		decode_payload || fail "invalid manifest from $manifest"
		latest="$(payload_field version)"
		url="$(archive_field url)"
		size="$(archive_field size)"
		signature="$(archive_field signature)"
		[ -n "$url" ] || fail "$manifest names no archive"
		[ -n "$size" ] && [ -n "$signature" ] || fail "$manifest names no signed archive"
		echo "Downloading $app_name $latest"
		archive="$work/$name.tar.gz"
		fetch "$url" >"$archive" || fail "could not download $url"
		[ "$(wc -c <"$archive" | tr -d ' ')" = "$size" ] || fail "$url does not have the size of the manifest: not installing it"
		verify_openssl "$archive" "$signature" || fail "$url is not signed with the key of $app_name: not installing it"
{{- end}}
	fi
	[ -f "$archive" ] || fail "no archive at $archive"
	tar -tzf "$archive" >"$work/names" 2>/dev/null || fail "$archive is not a .tar.gz archive"
	if grep -Eq '^/|(^|/)\.\.(/|$)' "$work/names"; then
		fail "$archive holds files outside the app"
	fi

	echo "Installing $app_name in $app_dir"
	rm -rf "$staging"
	mkdir -p "$staging"
	tar -xzf "$archive" -C "$staging"
	[ -f "$staging/$name" ] && [ -x "$staging/$name" ] || fail "$archive is not $app_name: it holds no $name"
	# Replace the app as a whole, so that no file of another version stays.
	rm -rf "$app_dir"
	mv "$staging" "$app_dir"

	mkdir -p "$data_home/applications"
	# Earlier install scripts linked a command named after the app. Links of
	# other names an earlier version had stay, as one the user made would:
	# --uninstall removes them.
	case " $command_names " in
	*" $name "*) ;;
	*) if ours "$bin_dir/$name"; then rm -f "$bin_dir/$name"; fi ;;
	esac
	run=
	for command_name in $command_names; do
		bin_link="$bin_dir/$command_name"
		if { [ -e "$bin_link" ] || [ -L "$bin_link" ]; } && ! ours "$bin_link"; then
			echo "Leaving $bin_link alone: it is not $app_name's" >&2
		else
			mkdir -p "$bin_dir"
			ln -sf "$app_dir/$name" "$bin_link"
			# The app runs by the first command linked.
			if [ -z "$run" ]; then
				run=$bin_link
				if [ "$(command -v "$command_name" || true)" = "$bin_link" ]; then
					run=$command_name
				fi
			fi
		fi
	done
	# The entry of the app, running it and showing its icon by their paths,
	# which updates of the app register again the same way.
	if [ -f "$app_dir/$name.desktop" ]; then
		while IFS= read -r line || [ -n "$line" ]; do
			case "$line" in
			"Exec=$name" | "Exec=$name "*) line="Exec=\"$app_dir/$name\"${line#"Exec=$name"}" ;;
			"Icon=$name") if [ -f "$app_dir/$name.png" ]; then line="Icon=$app_dir/$name.png"; fi ;;
			esac
			printf '%s\n' "$line"
		done <"$app_dir/$name.desktop" >"$desktop_file"
	fi
	if [ -f "$app_dir/$name.xml" ]; then
		mkdir -p "$data_home/mime/packages"
		cp "$app_dir/$name.xml" "$mime_file"
	else
		rm -f "$mime_file"
	fi
	update_databases

	if [ -n "$run" ]; then
		echo "Installed $app_name: open it from the applications menu, or run $run"
	else
		echo "Installed $app_name: open it from the applications menu"
	fi
}
{{- if .Updates}}

# latest_tag prints the tag of the newest release tagged $tag_prefix that is
# neither a draft nor a prerelease. The GitHub API lists releases newest
# first, each with its tag_name, draft and prerelease fields in that order.
latest_tag() {
	page=1
	while [ "$page" -le 10 ]; do
		fetch "$api&page=$page" >"$work/releases.json" || return 1
		tr -d '\n' <"$work/releases.json" |
			grep -oE '"(tag_name|draft|prerelease)": *("[^"]*"|true|false)' >"$work/fields" || true
		tag="$(awk -v prefix="$tag_prefix" '
			/^"tag_name"/ { sub(/^"tag_name": *"/, ""); sub(/"$/, ""); tag = $0; draft = 0; next }
			/^"draft"/ { draft = /true$/; next }
			/^"prerelease"/ { if (!draft && /false$/ && index(tag, prefix) == 1) { print tag; exit } }
		' "$work/fields")"
		if [ -n "$tag" ]; then
			echo "$tag"
			return
		fi
		# A full page leads to the next.
		[ "$(grep -c '^"tag_name"' "$work/fields" || true)" -ge 100 ] || return 0
		page=$((page + 1))
	done
}

# decode_payload extracts base64 "payload" from SignedManifest v2 into $work/payload.json.
decode_payload() {
	b64="$(sed -n 's/^  "payload": "\([^"]*\)",\{0,1\}$/\1/p' "$work/update.json" | head -n 1)"
	[ -n "$b64" ] || return 1
	if [ "$(uname -s)" = Darwin ]; then
		printf '%s' "$b64" | base64 -D >"$work/payload.json" 2>/dev/null || return 1
	elif command -v openssl >/dev/null 2>&1; then
		printf '%s' "$b64" | openssl base64 -d -A >"$work/payload.json" 2>/dev/null || return 1
	else
		printf '%s' "$b64" | base64 -d >"$work/payload.json" 2>/dev/null || return 1
	fi
}

payload_field() {
	awk -v k="\"$1\":" '
		{
			i = index($0, k)
			if (i == 0) exit 1
			s = substr($0, i + length(k))
			if (substr(s, 1, 1) == "\"") {
				s = substr(s, 2)
				j = index(s, "\"")
				print substr(s, 1, j - 1)
			} else {
				match(s, /^[0-9]+/)
				print substr(s, RSTART, RLENGTH)
			}
		}
	' "$work/payload.json"
}

archive_field() {
	awk -v k="\"$1\":" '
		{
			p = index($0, "\"legacyArchive\":{")
			if (p == 0) p = index($0, "\"artifact\":{")
			if (p == 0) exit 1
			blk = substr($0, p)
			end = index(blk, "}")
			if (end > 0) blk = substr(blk, 1, end)
			i = index(blk, k)
			if (i == 0) exit 1
			s = substr(blk, i + length(k))
			if (substr(s, 1, 1) == "\"") {
				s = substr(s, 2)
				j = index(s, "\"")
				print substr(s, 1, j - 1)
			} else {
				match(s, /^[0-9]+/)
				print substr(s, RSTART, RLENGTH)
			}
		}
	' "$work/payload.json"
}

verify_openssl() {
	printf '%s' "$public_key" | grep -Eq '^[A-Za-z0-9+/]{43}=$' || return 1
	printf '%s' "$2" | grep -Eq '^[A-Za-z0-9+/]{86}==$' || return 1
	{
		printf 'mygo-update-artifact-v2\n'
		openssl dgst -sha256 -binary <"$1"
	} >"$work/digest" || return 1
	ed25519_verify "MCowBQYDK2VwAyEA$public_key" "$2" "$work/digest"
}

# ed25519_verify checks with OpenSSL that the file $3 has the Ed25519
# signature $2, in base64, by the public key $1, in base64 DER.
ed25519_verify() {
	printf '%s\n' '-----BEGIN PUBLIC KEY-----' "$1" '-----END PUBLIC KEY-----' >"$work/key.pem"
	printf '%s' "$2" | openssl base64 -d -A >"$work/signature" 2>/dev/null || return 1
	[ "$(wc -c <"$work/signature" | tr -d ' ')" = 64 ] || return 1
	openssl pkeyutl -verify -pubin -inkey "$work/key.pem" -rawin -in "$3" -sigfile "$work/signature" >/dev/null 2>&1
}

# has_ed25519 tells whether openssl checks Ed25519 signatures, as OpenSSL
# 1.1.1 and LibreSSL do not, with test 2 of RFC 8032, whose message is the
# byte 0x72.
has_ed25519() {
	command -v openssl >/dev/null 2>&1 || return 1
	printf r >"$work/rfc8032"
	ed25519_verify MCowBQYDK2VwAyEAPUAXw+hDiVqStwqnTRt+vJyYLM8uxJaMwM1V8Sr0Zgw= \
		kqAJqfDUyrhyDoILX2QlQKKye1QWUD+Ps3YiI+vbadoIWsHkPhWZbkWPNhPQ8R2MOHsurrQwKu6wDSkWErsMAA== "$work/rfc8032"
}
{{- end}}

uninstall() {
	[ -d "$app_dir" ] || fail "$app_name is not installed in $app_dir"
	# Only what install.sh and the app made: a package of the system may
	# install the same names. Links of names the app no longer has go too.
	for link in "$bin_dir"/*; do
		if ours "$link"; then
			rm -f "$link"
		fi
	done
	if [ -f "$desktop_file" ] && grep -qF "$app_dir/" "$desktop_file"; then
		rm -f "$desktop_file" "$mime_file"
	fi
	for entry in "$data_home"/applications/*.url-handler.desktop; do
		if [ -f "$entry" ] && grep -qF "$app_dir/" "$entry"; then
			rm -f "$entry"
		fi
	done
	rm -rf "$app_dir"
	update_databases
	echo "Uninstalled $app_name. Its settings and data are still in place."
}

# ours tells whether $1 is the link to the app that install.sh makes.
ours() {
	[ -L "$1" ] && [ "$(readlink "$1")" = "$app_dir/$name" ]
}

update_databases() {
	if command -v update-desktop-database >/dev/null 2>&1; then
		update-desktop-database "$data_home/applications" >/dev/null 2>&1 || true
	fi
	if [ -d "$data_home/mime/packages" ] && command -v update-mime-database >/dev/null 2>&1; then
		update-mime-database "$data_home/mime" >/dev/null 2>&1 || true
	fi
}

main "$@"
`))
