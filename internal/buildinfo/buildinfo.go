package buildinfo

import "strings"

var (
	Version   = "dev"
	BuildTime = ""
)

const (
	ReleaseRepository = "xuzhougeng/citebox"
	LatestReleaseAPI  = "https://api.github.com/repos/" + ReleaseRepository + "/releases/latest"
	ReleasesPageURL   = "https://github.com/" + ReleaseRepository + "/releases/latest"
)

func CurrentVersion() string {
	version := strings.TrimSpace(Version)
	if version == "" {
		return "dev"
	}
	return version
}

func CurrentBuildTime() string {
	return strings.TrimSpace(BuildTime)
}
