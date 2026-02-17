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
		winPath := wslToWindows(path)
		return exec.Command("explorer.exe", winPath).Start()
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
		return openBrowserWSL(url)
	case runtime.GOOS == "darwin":
		return exec.Command("open", url).Start()
	case runtime.GOOS == "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

// openBrowserWSL tries multiple approaches to open a URL from WSL.
func openBrowserWSL(url string) error {
	// Try wslview first (from wslu package)
	if path, err := exec.LookPath("wslview"); err == nil {
		return exec.Command(path, url).Start()
	}

	// Try sensible-browser
	if path, err := exec.LookPath("sensible-browser"); err == nil {
		return exec.Command(path, url).Start()
	}

	// Fall back to cmd.exe /c start (works on all WSL setups)
	// The empty string "" as second arg prevents cmd.exe from treating
	// URLs with & as command separators
	return exec.Command("cmd.exe", "/c", "start", "", url).Start()
}

var wslCached *bool

func isWSL() bool {
	if wslCached != nil {
		return *wslCached
	}
	result := false
	if runtime.GOOS == "linux" {
		out, err := exec.Command("uname", "-r").Output()
		if err == nil {
			lower := strings.ToLower(string(out))
			result = strings.Contains(lower, "microsoft") || strings.Contains(lower, "wsl")
		}
	}
	wslCached = &result
	return result
}

func wslToWindows(path string) string {
	out, err := exec.Command("wslpath", "-w", path).Output()
	if err != nil {
		return path
	}
	return strings.TrimSpace(string(out))
}
