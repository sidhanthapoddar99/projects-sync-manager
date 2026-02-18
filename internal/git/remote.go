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

// HTTPSToSSH converts a git HTTPS URL to SSH format.
// e.g. https://github.com/user/repo -> git@github.com:user/repo.git
func HTTPSToSSH(url string) string {
	url = strings.TrimSuffix(url, "/")
	url = strings.TrimSuffix(url, ".git")

	// Match https://host/user/repo
	for _, prefix := range []string{"https://", "http://"} {
		if strings.HasPrefix(url, prefix) {
			rest := strings.TrimPrefix(url, prefix)
			parts := strings.SplitN(rest, "/", 2)
			if len(parts) == 2 {
				return fmt.Sprintf("git@%s:%s.git", parts[0], parts[1])
			}
		}
	}
	return url
}

// ExtractHost extracts the hostname from a git URL (HTTPS or SSH).
func ExtractHost(url string) string {
	if matches := sshURLPattern.FindStringSubmatch(url); matches != nil {
		return matches[1]
	}
	for _, prefix := range []string{"https://", "http://"} {
		if strings.HasPrefix(url, prefix) {
			rest := strings.TrimPrefix(url, prefix)
			parts := strings.SplitN(rest, "/", 2)
			if len(parts) >= 1 {
				return parts[0]
			}
		}
	}
	return ""
}

// CheckSSHAccess tests if SSH access is available for a given host.
// Returns true if `ssh -T git@host` succeeds (exit code 0 or 1 — GitHub returns 1 with "Hi user!").
func CheckSSHAccess(host string) bool {
	cmd := exec.Command("ssh", "-T", "-o", "StrictHostKeyChecking=accept-new", "-o", "ConnectTimeout=5", fmt.Sprintf("git@%s", host))
	err := cmd.Run()
	if err == nil {
		return true
	}
	// GitHub/GitLab return exit code 1 with a greeting — that still means SSH works
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode() == 1
	}
	return false
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
