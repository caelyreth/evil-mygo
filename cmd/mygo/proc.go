package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// shellCommand runs a command line such as "bun run dev" through the shell.
func shellCommand(dir, line string, env ...string) *exec.Cmd {
	cmd := shell(line)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(), env...)
	setProcessGroup(cmd)
	return cmd
}

// command runs a tool quietly; its output becomes part of the error when
// it fails.
func command(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %v\n%s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// run runs a command line as the step t, its output labeled with label.
func run(t *task, label, dir, line string, env ...string) error {
	t.set(line)
	cmd := shellCommand(dir, line, env...)
	out, errOut := labeledOutput(cmd, label)
	err := waited(cmd.Run())
	out.flush()
	errOut.flush()
	if err != nil {
		return fmt.Errorf("%s: %w", line, err)
	}
	return nil
}

// labeledOutput labels what cmd prints with label, in colors when the
// terminal shows them.
func labeledOutput(cmd *exec.Cmd, label string) (out, errOut *labeled) {
	out, errOut = con.output(label, os.Stdout), con.output(label, os.Stderr)
	cmd.Stdout, cmd.Stderr = out, errOut
	cmd.WaitDelay = waitDelay
	if out.color && os.Getenv("NO_COLOR") == "" {
		cmd.Env = append(cmd.Env, "FORCE_COLOR=1") // it now writes to a pipe
	}
	return out, errOut
}

// goCommand runs the go tool.
func goCommand(dir string, env []string, args ...string) *exec.Cmd {
	return goCommandContext(context.Background(), dir, env, args...)
}

func goCommandContext(ctx context.Context, dir string, env []string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	cmd.Env = os.Environ()
	// MyGo needs no cgo by default, but an app's dependencies may.
	if os.Getenv("CGO_ENABLED") == "" {
		cmd.Env = append(cmd.Env, "CGO_ENABLED=0")
	}
	cmd.Env = append(cmd.Env, env...)
	return cmd
}

// buildBinary compiles the app package into out.
func buildBinary(c *Config, out string, env []string, flags ...string) error {
	return goBuild(context.Background(), c, nil, out, env, flags...)
}

// goBuild compiles the app package into out, showing on t the packages it
// compiles, which go build -v lists.
func goBuild(ctx context.Context, c *Config, t *task, out string, env []string, flags ...string) error {
	args := append([]string{"build", "-v", "-o", out}, flags...)
	args = append(args, c.Main)
	cmd := goCommandContext(ctx, c.root, env, args...)
	p := &goProgress{t: t}
	cmd.Stdout, cmd.Stderr = p, p
	cmd.WaitDelay = waitDelay
	err := waited(cmd.Run())
	p.flush()
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return &goBuildError{out: p.diag.String(), err: err}
	}
	if p.diag.Len() > 0 { // warnings
		l := con.output("go", os.Stderr)
		_, _ = l.Write([]byte(p.diag.String()))
		l.flush()
	}
	return nil
}

// goBuildError is a go build that failed, with what it printed.
type goBuildError struct {
	out string
	err error
}

func (e *goBuildError) Error() string {
	if out := strings.TrimSpace(e.out); out != "" {
		return "go build failed:\n" + out
	}
	return "go build failed: " + e.err.Error()
}

func (e *goBuildError) Unwrap() error { return e.err }

// goProgress reads what go build -v prints: the import paths of the
// packages it compiles, which it shows on t, the modules it downloads, and
// errors, which it keeps.
type goProgress struct {
	t        *task
	buf      []byte
	packages int
	diag     strings.Builder
}

func (p *goProgress) Write(b []byte) (int, error) {
	p.buf = append(p.buf, b...)
	for {
		i := bytes.IndexByte(p.buf, '\n')
		if i < 0 {
			break
		}
		p.line(strings.TrimRight(string(p.buf[:i]), "\r"))
		p.buf = p.buf[i+1:]
	}
	return len(b), nil
}

func (p *goProgress) flush() {
	if len(p.buf) > 0 {
		p.line(string(p.buf))
		p.buf = nil
	}
}

func (p *goProgress) line(s string) {
	switch {
	case strings.HasPrefix(s, "go: downloading "):
		p.t.set("downloading " + strings.TrimPrefix(s, "go: downloading "))
	case isImportPath(s):
		p.packages++
		n := "1 package"
		if p.packages > 1 {
			n = fmt.Sprintf("%d packages", p.packages)
		}
		p.t.set(n + " · " + s)
	default:
		p.diag.WriteString(s + "\n")
	}
}

// isImportPath reports whether a line of go build -v is the import path of
// a package rather than a message, which has spaces or colons.
func isImportPath(s string) bool {
	if s == "" || s[0] == '#' || s[0] == '.' {
		return false
	}
	for _, r := range s {
		switch {
		case 'a' <= r && r <= 'z', 'A' <= r && r <= 'Z', '0' <= r && r <= '9':
		case strings.ContainsRune("-._~/+", r):
		default:
			return false
		}
	}
	return true
}

func tempBinary(name string) string {
	dir := filepath.Join(os.TempDir(), "mygo-dev")
	_ = os.MkdirAll(dir, 0o755)
	name = strings.ReplaceAll(name, " ", "-")
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(dir, fmt.Sprintf("%s-%d", name, os.Getpid())+filepath.Ext(name))
}

// waitDelay is how long Wait waits for the output of a command that exited
// when processes it started hold it, as a build command may leave a server
// running.
const waitDelay = time.Second

// waited is the error of Wait, which WaitDelay stopping it from waiting
// for the output does not make one.
func waited(err error) error {
	if errors.Is(err, exec.ErrWaitDelay) {
		return nil
	}
	return err
}
