package scanner

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const ignoreFileName = ".psmignore"

// Default patterns compiled as regexes - these are always active
var defaultIgnorePatterns = []string{
	// Hidden directories
	`^\.`,
	// JS/Node
	`^node_modules$`,
	`^\.npm$`,
	`^\.nvm$`,
	`^\.yarn$`,
	`^\.pnpm-store$`,
	`^bower_components$`,
	// Build outputs
	`^build$`,
	`^dist$`,
	`^out$`,
	`^target$`,
	`^bin$`,
	`^obj$`,
	`^_build$`,
	`^cmake-build-`,
	// Python
	`^__pycache__$`,
	`^\.venv$`,
	`^venv$`,
	`^env$`,
	`^\.env$`,
	`^\.eggs$`,
	`.*\.egg-info$`,
	`^\.tox$`,
	`^\.mypy_cache$`,
	`^\.pytest_cache$`,
	// Go
	`^vendor$`,
	// Rust
	`^target$`,
	// Java/Gradle/Maven
	`^\.gradle$`,
	`^\.mvn$`,
	// Ruby
	`^\.bundle$`,
	// IDE / editor
	`^\.idea$`,
	`^\.vscode$`,
	`^\.vs$`,
	`^\.eclipse$`,
	// OS junk
	`^\.Trash`,
	`^\.cache$`,
	`^\.local$`,
	`^\.config$`,
	// Docker
	`^\.docker$`,
	// Terraform
	`^\.terraform$`,
	// Misc
	`^coverage$`,
	`^\.nyc_output$`,
	`^\.next$`,
	`^\.nuxt$`,
	`^\.svelte-kit$`,
	`^\.turbo$`,
	`^\.parcel-cache$`,
	`^tmp$`,
	`^temp$`,
	`^logs$`,
}

// ignoreList holds compiled regexes for directory matching.
type ignoreList struct {
	patterns []*regexp.Regexp
}

// loadIgnoreList loads patterns from the .psmignore file at rootPath and
// merges them with the built-in defaults. Returns a compiled ignore list.
func loadIgnoreList(rootPath string) *ignoreList {
	il := &ignoreList{}

	// Compile defaults
	for _, p := range defaultIgnorePatterns {
		if re, err := regexp.Compile(p); err == nil {
			il.patterns = append(il.patterns, re)
		}
	}

	// Load user file
	filePath := filepath.Join(rootPath, ignoreFileName)
	f, err := os.Open(filePath)
	if err != nil {
		return il // no user file, defaults only
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if re, err := regexp.Compile(line); err == nil {
			il.patterns = append(il.patterns, re)
		}
	}

	return il
}

// shouldIgnore returns true if the directory name matches any ignore pattern.
func (il *ignoreList) shouldIgnore(dirName string) bool {
	for _, re := range il.patterns {
		if re.MatchString(dirName) {
			return true
		}
	}
	return false
}

// GenerateDefaultIgnoreFile writes a .psmignore with the default patterns
// (commented with explanations) to the given root path.
func GenerateDefaultIgnoreFile(rootPath string) error {
	filePath := filepath.Join(rootPath, ignoreFileName)
	if _, err := os.Stat(filePath); err == nil {
		return nil // already exists, don't overwrite
	}

	content := `# PSM Ignore File
# Directories matching these regex patterns are skipped during scanning.
# One pattern per line. Lines starting with # are comments.
# These are in addition to the built-in defaults.
#
# Built-in defaults already cover:
#   Hidden dirs (.*)         node_modules    build/dist/out/target
#   __pycache__/venv         vendor          .gradle/.mvn
#   .idea/.vscode            .cache/.local   coverage/.next/.nuxt
#
# Add your custom patterns below:

# Example: skip any directory named "archive" or "backup"
# ^archive$
# ^backup$
# ^old-
`
	return os.WriteFile(filePath, []byte(content), 0644)
}
