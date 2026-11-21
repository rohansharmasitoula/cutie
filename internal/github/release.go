package github

import (
	"strings"

	"github.com/rohansharmasitoula/cutie/internal/utils"
)

// LatestRelease returns the tag of repo's (owner/name) latest GitHub release, e.g. v0.2.0.
func LatestRelease(repo string) (string, error) {
	out, err := utils.Run("gh", "api", "repos/"+repo+"/releases/latest", "--jq", ".tag_name")
	return strings.TrimSpace(string(out)), err
}
