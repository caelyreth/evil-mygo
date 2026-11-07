package unsupported

import "github.com/caelyreth/evil-mygo/internal/platform"

func (*Backend) NewFileWatch(func(platform.FileWatchNotice)) (platform.FileWatch, error) {
	return nil, platform.ErrUnsupported
}

func (*Backend) ReplaceFile(target, replacement string) error { return platform.ErrUnsupported }
