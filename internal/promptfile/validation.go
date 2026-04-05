package promptfile

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	majorOnly = regexp.MustCompile(`^\d+$`)
	semver    = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	digestPin = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

// ValidatePath rejects path traversal, absolute paths, and backslashes.
func ValidatePath(path string) error {
	if strings.Contains(path, "..") {
		return fmt.Errorf("path %q contains ..", path)
	}
	if strings.HasPrefix(path, "/") || strings.HasPrefix(path, "~") {
		return fmt.Errorf("path %q is absolute", path)
	}
	if strings.Contains(path, "\\") {
		return fmt.Errorf("path %q contains backslash", path)
	}
	return nil
}

// ValidateVersion accepts: latest, N (major), N.N.N (semver), sha256:<hex>.
func ValidateVersion(version string) error {
	if version == "latest" {
		return nil
	}
	if majorOnly.MatchString(version) {
		return nil
	}
	if semver.MatchString(version) {
		return nil
	}
	if digestPin.MatchString(version) {
		return nil
	}
	return fmt.Errorf("unsupported version format %q", version)
}
