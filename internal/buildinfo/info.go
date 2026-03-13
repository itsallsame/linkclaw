package buildinfo

import (
	"runtime/debug"
	"strings"
)

var (
	Version   = "dev"
	Commit    = ""
	BuildTime = ""
)

type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
	Dirty     bool   `json:"dirty"`
}

func Current() Info {
	info := Info{
		Version:   normalizeVersion(Version),
		Commit:    strings.TrimSpace(Commit),
		BuildTime: strings.TrimSpace(BuildTime),
	}

	if runtimeInfo, ok := debug.ReadBuildInfo(); ok {
		if shouldUseRuntimeVersion(info.Version) && runtimeInfo.Main.Version != "" && runtimeInfo.Main.Version != "(devel)" {
			info.Version = normalizeVersion(runtimeInfo.Main.Version)
		}
		for _, setting := range runtimeInfo.Settings {
			switch setting.Key {
			case "vcs.revision":
				if info.Commit == "" {
					info.Commit = strings.TrimSpace(setting.Value)
				}
			case "vcs.time":
				if info.BuildTime == "" {
					info.BuildTime = strings.TrimSpace(setting.Value)
				}
			case "vcs.modified":
				info.Dirty = setting.Value == "true"
			}
		}
	}

	if info.Version == "" {
		info.Version = "dev"
	}
	if info.Commit == "" {
		info.Commit = "unknown"
	}
	if info.BuildTime == "" {
		info.BuildTime = "unknown"
	}

	return info
}

func shouldUseRuntimeVersion(version string) bool {
	switch strings.TrimSpace(version) {
	case "", "dev", "unknown":
		return true
	default:
		return false
	}
}

func normalizeVersion(version string) string {
	trimmed := strings.TrimSpace(version)
	if trimmed == "" {
		return ""
	}
	if len(trimmed) > 1 && trimmed[0] == 'v' && trimmed[1] >= '0' && trimmed[1] <= '9' {
		return trimmed[1:]
	}
	return trimmed
}
