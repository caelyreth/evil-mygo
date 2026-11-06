package webgpu

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// RuntimePath returns the wgpu-native library the process will load, if
// one can be located. WGPU_NATIVE_PATH wins, then lib/ next to the
// executable or in an ancestor of the working directory, then the
// platform loader's search path.
func RuntimePath() (string, bool) {
	if path := os.Getenv("WGPU_NATIVE_PATH"); path != "" {
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
		return path, false
	}
	name := libraryName()
	if executable, err := os.Executable(); err == nil {
		dir := filepath.Dir(executable)
		for _, candidate := range []string{
			filepath.Join(dir, name),
			filepath.Join(dir, "lib", name),
		} {
			if exists(candidate) {
				return candidate, true
			}
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		for {
			if candidate := filepath.Join(cwd, "lib", name); exists(candidate) {
				return candidate, true
			}
			parent := filepath.Dir(cwd)
			if parent == cwd {
				break
			}
			cwd = parent
		}
	}
	return name, false
}

func prepareRuntime() error {
	if os.Getenv("WGPU_BACKEND") == "" && runtime.GOOS == "darwin" {
		// go-webgpu maps this path to Metal through wgpu-native.
		if err := os.Setenv("WGPU_BACKEND", "metal"); err != nil {
			return fmt.Errorf("webgpu: select Metal backend: %w", err)
		}
	}
	if os.Getenv("WGPU_NATIVE_PATH") != "" {
		return nil
	}
	path, ok := RuntimePath()
	if !ok {
		// Let wgpu's platform loader try PATH and DYLD_LIBRARY_PATH.
		return nil
	}
	if err := os.Setenv("WGPU_NATIVE_PATH", path); err != nil {
		return fmt.Errorf("webgpu: set WGPU_NATIVE_PATH: %w", err)
	}
	return nil
}

func libraryName() string {
	switch runtime.GOOS {
	case "windows":
		return "wgpu_native.dll"
	case "darwin":
		return "libwgpu_native.dylib"
	default:
		return "libwgpu_native.so"
	}
}

func exists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
