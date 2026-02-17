package git

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

var sshURLPattern = regexp.MustCompile(`^git@([^:]+):(.+?)(?:\.git)?$`)

// Remote represents a git remote.
type Remote struct {
	Name string
	URL  string
}

// GetRemotes returns all remotes for a git repository.
func GetRemotes(repoPath string) ([]Remote, error) {
	out, err := runGit(repoPath, "remote", "-v")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return nil, nil
	}

	seen := make(map[string]bool)
	var remotes []Remote
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		name := parts[0]
		if seen[name] {
			continue
		}
		seen[name] = true
		remotes = append(remotes, Remote{Name: name, URL: parts[1]})
	}
	return remotes, nil
}

// GetOriginURL returns the URL of the "origin" remote, or empty if none.
func GetOriginURL(repoPath string) string {
	remotes, err := GetRemotes(repoPath)
	if err != nil {
		return ""
	}
	for _, r := range remotes {
		if r.Name == "origin" {
			return r.URL
		}
	}
	if len(remotes) > 0 {
		return remotes[0].URL
	}
	return ""
}

// SSHToHTTPS converts a git SSH URL to HTTPS.
// e.g. git@github.com:user/repo.git -> https://github.com/user/repo
func SSHToHTTPS(url string) string {
	if matches := sshURLPattern.FindStringSubmatch(url); matches != nil {
		return fmt.Sprintf("https://%s/%s", matches[1], matches[2])
	}
	// Already HTTPS or other format, strip .git suffix
	url = strings.TrimSuffix(url, ".git")
	return url
}

// runGit executes a git command in the given directory and returns stdout.
// It passes -c safe.directory=* to handle copied/moved repositories that
// git considers "unsafe" due to ownership mismatch.
func runGit(dir string, args ...string) (string, error) {
	fullArgs := append([]string{"-c", "safe.directory=*"}, args...)
	cmd := exec.Command("git", fullArgs...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), string(exitErr.Stderr))
		}
		return "", err
	}
	return string(out), nil
}
