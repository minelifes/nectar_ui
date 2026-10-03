//go:build darwin

package gogpu

import "github.com/minelifes/nectar_ui/internal/gogpu/internal/platform/darwin"

func runInFramePool(fn func()) {
	darwin.RunInFramePool(fn)
}
