package opener

import (
	"os/exec"
	"runtime"
	"strings"
)

// OpenInVSCode opens a directory in VS Code.
func OpenInVSCode(path string) error {
	return exec.Command("code", path).Start()
}

// OpenInExplorer opens a directory in the system file explorer.
func OpenInExplorer(path string) error {
	switch {
	case isWSL():
		return exec.Command("explorer.exe", wslToWindows(path)).Start()
	case runtime.GOOS == "darwin":
		return exec.Command("open", path).Start()
	case runtime.GOOS == "windows":
		return exec.Command("explorer", path).Start()
	default: // linux
		return exec.Command("xdg-open", path).Start()
	}
}

// OpenInBrowser opens a URL in the default browser.
func OpenInBrowser(url string) error {
	switch {
	case isWSL():
		return exec.Command("wslview", url).Start()
	case runtime.GOOS == "darwin":
		return exec.Command("open", url).Start()
	case runtime.GOOS == "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

func isWSL() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	out, err := exec.Command("uname", "-r").Output()
	if err != nil {
		return false
	}
	lower := strings.ToLower(string(out))
	return strings.Contains(lower, "microsoft") || strings.Contains(lower, "wsl")
}

func wslToWindows(path string) string {
	out, err := exec.Command("wslpath", "-w", path).Output()
	if err != nil {
		return path
	}
	return strings.TrimSpace(string(out))
}
