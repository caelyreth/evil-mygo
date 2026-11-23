// Command mygo is the MyGo development tool: it scaffolds projects, runs
// native UI apps in development, and builds packaged production apps.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

const usage = `mygo is the tool for MyGo desktop applications.

Usage:

	mygo <command> [arguments]

Commands:

	init [dir]           create a new project
	vet [dir]            run Go vet and check native UI build lifetimes
	generate             write the accessors of the app's catalogs of messages
	i18n check [dir]     check the app's catalogs of messages
	dev                  run a development build with live reload
	build                build production apps (a .app and a .dmg on macOS)
	native <command>     make, check, call and build native modules in Swift,
	                     Kotlin, Rust or Zig, experimental (mygo native help)
	keygen               create the key pair that signs updates
	devices              list the simulators, emulators and devices mygo dev -device takes
	publish              upload a build to the App Store or Google Play
	doctor               check that the development environment is ready
	version              print the MyGo version

Run "mygo <command> -h" for the flags of a command.
`

const version = "0.10.0"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	// Ctrl+C leaves the line of a step in progress, not its spinner. mygo dev
	// stops its app first.
	if cmd == "build" || cmd == "native" || cmd == "generate" || cmd == "gen" || cmd == "init" || cmd == "publish" {
		go func() {
			sig := make(chan os.Signal, 1)
			signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
			<-sig
			con.interrupted()
			os.Exit(130)
		}()
	}
	var err error
	switch cmd {
	case "init":
		err = runInit(args)
	case "vet":
		err = runVetUI(args)
	case "generate", "gen":
		err = runGenerate(args)
	case "dev":
		err = runDev(args)
	case "i18n":
		err = runI18n(args)
	case "build":
		err = runBuild(args)
	case "native":
		err = runNative(args)
	case "doctor":
		err = runDoctor(args)
	case "devices":
		err = runDevices(args)
	case "publish":
		err = runPublish(args)
	case "keygen":
		err = runKeygen(args)
	case "sign-uninstaller": // run by makensis, see uninstallerSigning
		err = runSignUninstaller(args)
	case "version", "-v", "--version":
		fmt.Println("mygo", version)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "mygo: unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			con.fatal(err)
		}
		os.Exit(1)
	}
}

// newFlags creates a flag set that prints a usage line for a command.
func newFlags(name, args, summary string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mygo %s %s\n\n%s\n\nFlags:\n", name, args, summary)
		fs.PrintDefaults()
	}
	return fs
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
