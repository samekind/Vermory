package brand

import "runtime"

const (
	Name    = "Vermory"
	Slug    = "vermory"
	Tagline = "Governed Memory for AI"
)

var (
	Version   = "dev"
	Revision  = "unknown"
	BuildDate = "unknown"
)

type VersionInfo struct {
	Version   string `json:"version"`
	Revision  string `json:"revision"`
	BuildDate string `json:"build_date"`
	GoVersion string `json:"go_version"`
}

func Info() VersionInfo {
	return VersionInfo{
		Version:   Version,
		Revision:  Revision,
		BuildDate: BuildDate,
		GoVersion: runtime.Version(),
	}
}
