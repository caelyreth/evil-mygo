package nativegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

const languageSrc = `package language

import (
	"context"
	"io"
	"iter"
	"time"

	"github.com/egoist/mygo/native"
)

//mygo:native
type Analyzer interface {
	DetectLanguage(text string) (string, error)
	Hypotheses(text string, max int) (map[string]float64, error)
	Tokens(ctx context.Context, text string, unit Unit) iter.Seq2[Token, error]
	Embedding(ctx context.Context, language string) (Embedding, error)
	Suggest(ctx context.Context, e Embedding, word string, r Ranker) iter.Seq2[string, error]
	Count(text string, progress func(word string, index int)) int
	Ticks(interval time.Duration, f func(n int)) io.Closer
	Uppercase(text string, buf *native.Buffer) (int, error)
	Chunks(ctx context.Context, text string, size int) iter.Seq2[*native.Buffer, error]
	Sleep(ctx context.Context, d time.Duration) error
}

type Embedding interface {
	io.Closer
	Distance(a, b string) float64
	Neighbors(word string, n int) []string
}

type Ranker interface {
	Score(word string) float64
	Explain(ctx context.Context, word string) (string, error)
}

type Unit string

const (
	Word      Unit = "word"
	Sentence  Unit = "sentence"
	Paragraph Unit = "paragraph"
)

type Token struct {
	Start, End int
	Text       string
}

var (
	ErrUndetermined = native.NewError("undetermined")
	ErrNoEmbedding  = native.NewError("no_embedding")
)
`

func writeModulePair(t *testing.T) (langDir, placesDir, langImport string) {
	t.Helper()
	root, err := os.MkdirTemp("testdata", "pair-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	langDir = filepath.Join(root, "language")
	placesDir = filepath.Join(root, "places")
	for _, d := range []string{langDir, placesDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(langDir, "language.go"), []byte(languageSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	langImport = "github.com/egoist/mygo/internal/nativegen/" + filepath.ToSlash(langDir)
	placesSrc := `package places

import (
	"io"

	"github.com/egoist/mygo"
	"` + langImport + `"
	"github.com/egoist/mygo/native"
	"github.com/egoist/mygo/ui"
)

//mygo:native
type Map interface {
	native.View
	Show(p Place, span float64)
	Center() Place
	Pin(p Place, a language.Analyzer, text string)
	OnMove(f func(center Place)) io.Closer
}

//mygo:native
//mygo:mainthread
type Sharing interface {
	Share(win *mygo.Window, rect ui.Rect, text string)
	ShareFrom(view native.View, text string)
}

type Place struct {
	Latitude, Longitude float64
}
`
	if err := os.WriteFile(filepath.Join(placesDir, "places.go"), []byte(placesSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	return langDir, placesDir, langImport
}

func TestLoadModule(t *testing.T) {
	langDir, _, _ := writeModulePair(t)
	m, err := Load(langDir)
	if err != nil {
		t.Fatal(err)
	}
	if m.Package != "language" || m.Name != "Language" {
		t.Fatalf("module %s, %s", m.Package, m.Name)
	}
	byName := map[string]*Interface{}
	for _, it := range m.Interfaces {
		byName[it.Name] = it
	}
	if a := byName["Analyzer"]; a == nil || !a.Entry || a.ByGo {
		t.Errorf("Analyzer: %+v", a)
	}
	if e := byName["Embedding"]; e == nil || e.Entry || e.ByGo || !e.Closer {
		t.Errorf("Embedding, which the module returns and takes back: %+v", e)
	}
	if r := byName["Ranker"]; r == nil || !r.ByGo {
		t.Errorf("Ranker, which the module only takes: %+v", r)
	}
	if len(m.Errors) != 2 || m.Errors[1].Code != "no_embedding" || m.Errors[1].SwiftName() != "noEmbedding" {
		t.Errorf("errors: %+v", m.Errors)
	}
	if len(m.Enums) != 1 || len(m.Enums[0].Cases) != 3 || len(m.Structs) != 1 {
		t.Errorf("enums %+v, structs %+v", m.Enums, m.Structs)
	}
}

func TestModuleUsingAnother(t *testing.T) {
	langDir, placesDir, langImport := writeModulePair(t)
	if err := AddLang(langDir, "swift"); err != nil {
		t.Fatal(err)
	}
	m, err := Load(placesDir)
	if err != nil {
		t.Fatal(err)
	}
	dep := m.Deps[langImport]
	if dep == nil || dep.Name != "Language" {
		t.Fatalf("deps %v", m.Deps)
	}
	swift := string(GenerateSwift(m))
	for _, want := range []string{"import Language\n", "a: any Language.Analyzer", "public protocol Map: NativeView"} {
		if !strings.Contains(swift, want) {
			t.Errorf("the Swift lacks %q", want)
		}
	}
	src, err := GenerateGo(m, nil, SwiftPlatforms{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), `Deps: []string{"Language"}`) {
		t.Errorf("the Go does not load Language first:\n%s", src)
	}
}

// TestGeneratedUpToDate checks that generated files match what Stale expects.
func TestGeneratedUpToDate(t *testing.T) {
	langDir, placesDir, _ := writeModulePair(t)
	for _, dir := range []string{langDir, placesDir} {
		if _, err := Generate(dir); err != nil {
			t.Fatal(err)
		}
		stale, err := Stale(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(stale) > 0 {
			t.Fatalf("%s generated files unexpectedly stale: %v", dir, stale)
		}
	}
}

func TestNames(t *testing.T) {
	for in, want := range map[string]string{
		"DetectLanguage": "detectLanguage", "URLFor": "urlFor", "ID": "id",
		"HTTPServer": "httpServer", "X": "x", "already": "already",
	} {
		if got := lowerCamel(in); got != want {
			t.Errorf("lowerCamel(%q) = %q, want %q", in, got, want)
		}
	}
	if got := UpperCamel("media_player"); got != "MediaPlayer" {
		t.Errorf("UpperCamel = %q", got)
	}
	if got := swiftIdent("default"); got != "`default`" {
		t.Errorf("swiftIdent = %q", got)
	}
}

func TestDiagnostics(t *testing.T) {
	const head = "package bad\n\nimport (\n\t\"context\"\n\t\"io\"\n\t\"iter\"\n\t\"net/http\"\n)\n\nvar _ = context.Background\nvar _ io.Reader\nvar _ iter.Seq[int]\nvar _ http.Header\n\n"
	for _, c := range []struct{ src, want string }{
		{"type A interface{ M() }\n", "has no entry point"},
		{"//mygo:native\ntype A interface{ M(ch chan int) }\n", "bad.go:16: A.M uses chan int, which an implementation cannot use: return iter.Seq2[T, error]"},
		{"//mygo:native\ntype A interface{ M(v any) }\n", "uses any"},
		{"//mygo:native\ntype A interface{ M(h http.Header) }\n", "a type of another package"},
		{"//mygo:native\ntype A interface{ M(s string, ctx context.Context) }\n", "takes a context.Context after other parameters"},
		{"//mygo:native\ntype A interface{ M(ctx context.Context) iter.Seq2[int, string] }\n", "a stream's second value is its error"},
		{"//mygo:native\ntype A interface{ M() iter.Seq[int] }\n", "without taking a context.Context"},
		{"//mygo:native\ntype A interface{ M() (int, string) }\n", "the second not an error"},
		{"//mygo:native\ntype A interface{ M(m map[int]string) }\n", "maps have string keys"},
		{"//mygo:native\ntype A interface{ M(u U) }\ntype U string\n", "U has no constants"},
		{"//mygo:native\ntype A interface{ M(s S) }\ntype S struct{ Next *S }\n", "S contains itself"},
		{"//mygo:native\ntype A interface{ M(g G) }\ntype G interface{ N(f func()) }\n", "Go implements G"},
		{"//mygo:frob\ntype A interface{ M() }\n", "unknown directive //mygo:frob"},
		{"//mygo:native\ntype A interface{ M(_x int) }\n", "names a parameter _x: the generated code keeps names starting with _"},
		{"//mygo:native\ntype A interface{ M() }\nvar ErrX = native.NewError(\"\")\n", ""},
	} {
		dir := t.TempDir()
		src := head + c.src
		if strings.Contains(c.src, "native.") {
			src = strings.Replace(src, "import (\n", "import (\n\t\"github.com/egoist/mygo/native\"\n", 1)
		}
		if err := os.WriteFile(filepath.Join(dir, "bad.go"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := Load(dir)
		if c.want == "" {
			if err == nil {
				t.Errorf("%q: no error", c.src)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: error %v, want %q", c.src, err, c.want)
		}
	}
}

// TestScaffoldBuilds makes a module with its Swift package, whose stub
// implementation must compile against the generated declarations.
func TestScaffoldBuilds(t *testing.T) {
	if runtime.GOOS != "darwin" || testing.Short() {
		t.Skip("builds a Swift package, on macOS")
	}
	if _, err := exec.LookPath("swift"); err != nil {
		t.Skip("no Swift")
	}
	dir := filepath.Join(t.TempDir(), "greeting")
	if err := Scaffold(dir, "greeting", nil); err != nil {
		t.Fatal(err)
	}
	// Every kind of method, in the stub and the glue.
	src := strings.Replace(languageSrc, "package language", "package greeting", 1)
	os.Remove(filepath.Join(dir, "greeting.go"))
	if err := os.WriteFile(filepath.Join(dir, "greeting.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AddLang(dir, "swift"); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("swift", "build", "--package-path", filepath.Join(dir, "swift")).CombinedOutput()
	if err != nil {
		t.Fatalf("swift build: %v\n%s", err, out)
	}
	if strings.Contains(string(out), "warning:") {
		t.Errorf("swift build warns:\n%s", out)
	}
}

// TestScaffoldBuildsRustZig makes modules in Rust and Zig, a new one's
// and one whose names clash with the generated code's, whose stub
// implementations must compile against the generated declarations.
func TestScaffoldBuildsRustZig(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a crate and a Zig package")
	}
	clash, err := os.ReadFile("testdata/clash/clash.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, lang := range []string{"rust", "zig"} {
		tool := map[string]string{"rust": "cargo", "zig": "zig"}[lang]
		if _, err := exec.LookPath(tool); err != nil {
			t.Logf("no %s", tool)
			continue
		}
		for _, src := range []string{"", string(clash)} {
			dir := filepath.Join(t.TempDir(), "greeting")
			if err := Scaffold(dir, "greeting", nil); err != nil {
				t.Fatal(err)
			}
			if src != "" {
				src = regexp.MustCompile(`(?m)^package \w+`).ReplaceAllString(src, "package greeting")
				if err := os.WriteFile(filepath.Join(dir, "greeting.go"), []byte(src), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := AddLang(dir, lang); err != nil {
				t.Fatalf("%s: %v", lang, err)
			}
			cache := t.TempDir()
			var cmd *exec.Cmd
			if lang == "rust" {
				cmd = exec.Command("cargo", "build", "--manifest-path", filepath.Join(dir, "rust", "Cargo.toml"))
				cmd.Env = append(os.Environ(), "CARGO_TARGET_DIR="+cache)
			} else {
				cmd = exec.Command("zig", "build", "--prefix", filepath.Join(cache, "out"), "--cache-dir", filepath.Join(cache, "zig"))
				cmd.Dir = filepath.Join(dir, "zig")
			}
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("%s build of %q: %v\n%s", lang, firstLine(src), err, out)
			} else if strings.Contains(string(out), "warning") {
				t.Errorf("%s build of %q warns:\n%s", lang, firstLine(src), out)
			}
		}
	}
}

func firstLine(s string) string {
	l, _, _ := strings.Cut(s, "\n")
	return l
}

// TestNamesDontClash generates a module whose parameters are named as the
// generated code names its own identifiers and packages: its Go must vet,
// and its Swift build, where Swift is.
func TestNamesDontClash(t *testing.T) {
	dir, err := os.MkdirTemp("testdata", "clash-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	src, err := os.ReadFile("testdata/clash/clash.go")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "clash.go"), src, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(dir); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("go", "vet", "./"+dir).CombinedOutput()
	if err != nil {
		t.Fatalf("go vet of the generated Go: %v\n%s", err, out)
	}
	if runtime.GOOS != "darwin" || testing.Short() {
		return
	}
	if _, err := exec.LookPath("swift"); err != nil {
		return
	}
	if err := AddLang(dir, "swift"); err != nil {
		t.Fatal(err)
	}
	out, err = exec.Command("swift", "build", "--package-path", filepath.Join(dir, "swift")).CombinedOutput()
	if err != nil {
		t.Fatalf("swift build: %v\n%s", err, out)
	}
}

func TestKotlinNames(t *testing.T) {
	for in, want := range map[string]string{
		"github.com/acme/go-battery":                                "com.github.acme.go_battery",
		"github.com/egoist/mygo/native/x":                           "com.github.egoist.mygo.native_.x",
		"example.com/2d/package":                                    "com.example._2d.package_",
		"local/Mixed.Case":                                          "local.mixed_case",
		"github.com/egoist/mygo/plugins/sqlite/internal/sqlitecore": "com.github.egoist.mygo.plugins.sqlite.internal.sqlitecore",
	} {
		if got := KotlinPackage(in); got != want {
			t.Errorf("KotlinPackage(%q) = %q, want %q", in, got, want)
		}
	}
	if got := kotlinIdent("object"); got != "`object`" {
		t.Errorf("kotlinIdent = %q", got)
	}
}

// TestKotlinUpToDate checks the committed Kotlin of the modules that have
// it, which no test compiles here: CI builds it into an app.
func TestKotlinUpToDate(t *testing.T) {
	for _, dir := range []string{"../../native/internal/roundtrip"} {
		m, err := Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		want, err := GenerateKotlin(m)
		if err != nil {
			t.Fatal(err)
		}
		have, err := os.ReadFile(filepath.Join(KotlinDir(m), KotlinFile(m)))
		if err != nil {
			t.Fatal(err)
		}
		if string(have) != string(want) {
			t.Errorf("%s: the generated Kotlin is out of date: run go run ./cmd/mygo native check %s", dir, dir)
		}
		if !strings.Contains(string(want), "package "+KotlinPackage(m.ImportPath)+"\n") || !strings.Contains(string(want), "object MygoModule") {
			t.Errorf("%s: the Kotlin is not in its package, or has no MygoModule", dir)
		}
	}
}
