package admindeveloper

import (
	"os"
	"path/filepath"
)

// Match the 2.9.7 behavior: npm wins when both lock files are present.
func frontendUsesYarn(root string) bool {
	_, yarnErr := os.Stat(filepath.Join(root, "yarn.lock"))
	_, npmErr := os.Stat(filepath.Join(root, "package-lock.json"))
	return yarnErr == nil && os.IsNotExist(npmErr)
}
