package promptfile

import (
	"fmt"
	"strings"
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
