package ui

import "golang.org/x/mod/semver"

// cutieRepo is where cutie's releases are published.
const cutieRepo = "rohansharmasitoula/cutie"

// newerRelease reports whether release tag is newer than this build's version. Builds
// without a version (go build in a checkout without VCS info) never see a notice.
func newerRelease(current, tag string) bool {
	cur, latest := withV(current), withV(tag)
	return semver.IsValid(cur) && semver.IsValid(latest) && semver.Compare(latest, cur) > 0
}

// withV adds the v that release builds' versions (0.2.0) lack and Go's (v0.2.0) have.
func withV(v string) string {
	if v != "" && v[0] != 'v' {
		return "v" + v
	}
	return v
}

// updateHint says how to update this build: release binaries are downloaded again,
// everything else came from go install.
func updateHint(releaseBuild bool) string {
	if releaseBuild {
		return "download it from https://github.com/" + cutieRepo + "/releases/latest"
	}
	return "go install github.com/" + cutieRepo + "@latest"
}
