// TODO: consider replacing runtime.GOOS switch with build-tag-gated files (browser_darwin.go, etc.)
package browser

import (
	"fmt"
	"os/exec"
	"runtime"
)

func Open(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "windows":
		return exec.Command("cmd", "/c", "start", url).Start()
	default:
		return fmt.Errorf("unsupported platform %s", runtime.GOOS)
	}
}
