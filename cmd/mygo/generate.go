package main

import (
	"fmt"
	"runtime"
)

func runGenerate(args []string) error {
	fs := newFlags("generate", "[dir]", `With i18n in mygo.json, checks the catalogs of the app's messages and writes
their Go accessors in the catalogs' directory. Also generates native module
bindings when native modules are configured.`)
	if err := fs.Parse(args); err != nil {
		return err
	}
	c, err := loadConfig(dirArg(fs.Args()))
	if err != nil {
		return err
	}
	modules, err := appNativeModules(c, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	if len(modules) > 0 {
		if err := generateNative(c); err != nil {
			return err
		}
	}
	if c.I18n == nil {
		if len(modules) > 0 {
			return nil
		}
		return fmt.Errorf("nothing to generate: configure i18n or native modules in %s", c.configName())
	}
	if err := generateI18n(c); err != nil {
		return err
	}
	if dirs := compiledDirs(c); dirs != nil {
		checkCatalogImported(c, dirs)
	}
	return nil
}

func dirArg(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return "."
}
