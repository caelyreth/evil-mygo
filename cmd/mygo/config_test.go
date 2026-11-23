package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinuxMaintainer(t *testing.T) {
	for _, tc := range []struct{ config, want string }{
		{`{"name": "My App"}`, "My App"},
		{`{"name": "My App", "linux": {"maintainer": "Acme <dev@acme.test>"}}`, "Acme <dev@acme.test>"},
	} {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, jsonConfig), []byte(tc.config), 0o644)
		c, err := loadConfig(dir)
		if err != nil {
			t.Fatal(err)
		}
		if c.Linux.Maintainer != tc.want {
			t.Errorf("with %s, the maintainer is %q, want %q", tc.config, c.Linux.Maintainer, tc.want)
		}
	}
}

// TestWindowsDisplayName checks windows.displayName: the name Windows shows
// the app by, which falls back to the name, while the executable and the
// install folder keep the name's.
func TestWindowsDisplayName(t *testing.T) {
	load := func(config string) (*Config, error) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, jsonConfig), []byte(config), 0o644)
		return loadConfig(dir)
	}
	c, err := load(`{"name": "weather", "windows": {"displayName": "天气助手"}}`)
	if err != nil {
		t.Fatal(err)
	}
	if c.windowsName() != "天气助手" || c.shortcutName() != "天气助手" || c.executableName() != "weather" || c.slug() != "weather" {
		t.Errorf("windowsName %q, shortcutName %q, executableName %q, slug %q", c.windowsName(), c.shortcutName(), c.executableName(), c.slug())
	}
	if wl := windowsLanguages(c, nil); len(wl) != 1 || wl[0].name != "天气助手" {
		t.Errorf("the file properties name the app %+v", wl)
	}
	if wl := windowsLanguages(c, []appLanguage{{Tag: "en"}, {Tag: "de", Name: "Wetter"}, {Tag: "fr"}}); len(wl) != 3 || wl[0].name != "天气助手" || wl[1].name != "Wetter" || wl[2].name != "天气助手" {
		t.Errorf("the file properties name the app %+v", wl)
	}
	c.Updates = &Updates{URL: "https://dl.example.com/weather"}
	ps1 := installPS1(c)
	for _, want := range []string{"$appName = " + psQuote("天气助手") + "\n", "$exeName = 'weather.exe'\n", `('Programs\' + 'weather')`} {
		if !strings.Contains(ps1, want) {
			t.Errorf("install.ps1 has no %q", want)
		}
	}

	// Characters file names cannot hold leave the shortcuts' names only.
	c.Windows.DisplayName = ` Data: "Tools"? `
	if c.windowsName() != `Data: "Tools"?` || c.shortcutName() != "Data- -Tools--" {
		t.Errorf("windowsName %q, shortcutName %q", c.windowsName(), c.shortcutName())
	}
	// Without one, or a blank one, the name.
	for _, name := range []string{"", "  "} {
		c.Windows.DisplayName = name
		if c.windowsName() != "weather" || c.shortcutName() != "weather" {
			t.Errorf("%q: windowsName %q, shortcutName %q", name, c.windowsName(), c.shortcutName())
		}
	}
	if _, err := load(`{"name": "weather", "windows": {"displayName": "天气\n助手"}}`); err == nil || !strings.Contains(err.Error(), "windows.displayName") {
		t.Errorf("a display name that breaks the line loads: %v", err)
	}
}

// TestWindowsInstallDirName checks windows.installDirName: the name of the
// install folder, trimmed, which falls back to the name made a file name,
// and which must be one folder's name.
func TestWindowsInstallDirName(t *testing.T) {
	load := func(config string) (*Config, error) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, jsonConfig), []byte(config), 0o644)
		return loadConfig(dir)
	}
	for config, want := range map[string]string{
		`{"name": "智链"}`:           "智链",
		`{"name": "Data: Tools?"}`: "Data- Tools-",
		`{"name": "智链", "windows": {"installDirName": " ZhiLian "}}`:         "ZhiLian",
		`{"name": "智链", "windows": {"installDirName": "Wen Jing.Zhi-Lian"}}`: "Wen Jing.Zhi-Lian",
		`{"name": "智链", "windows": {"installDirName": "智链 2"}}`:              "智链 2",
	} {
		c, err := load(config)
		if err != nil {
			t.Errorf("%s: %v", config, err)
			continue
		}
		if got := c.installDirName(); got != want {
			t.Errorf("%s: the install folder is %q, want %q", config, got, want)
		}
		// Only the folder: the executable and the shortcuts keep the name.
		if c.Windows.InstallDirName != "" && (c.executableName() != "智链" || c.shortcutName() != "智链") {
			t.Errorf("%s: executableName %q, shortcutName %q", config, c.executableName(), c.shortcutName())
		}
		c.Updates = &Updates{URL: "https://dl.example.com/zhilian"}
		if ps1 := installPS1(c); !strings.Contains(ps1, `('Programs\' + `+psQuote(want)+`)`) {
			t.Errorf("%s: install.ps1 has no default folder %q", config, want)
		}
	}
	for _, name := range []string{" ", "a/b", `a\b`, `..\x`, "../x", ".", "..", "x.", "C:", "a*b", "a\tb", "con", "NUL.txt", "Com1"} {
		config, _ := json.Marshal(map[string]any{"name": "智链", "windows": map[string]string{"installDirName": name}})
		if _, err := load(string(config)); err == nil || !strings.Contains(err.Error(), "windows.installDirName") {
			t.Errorf("installDirName %q loads: %v", name, err)
		}
	}
}
