package version

import "runtime/debug"

const shortHash = 12

var value = ""

func String() string {
	if value != "" {
		return value
	}
	return fromBuild(debug.ReadBuildInfo())
}

func fromBuild(info *debug.BuildInfo, ok bool) string {
	if !ok {
		return "dev"
	}
	for _, setting := range info.Settings {
		if setting.Key != "vcs.revision" {
			continue
		}
		if len(setting.Value) > shortHash {
			return setting.Value[:shortHash]
		}
		if setting.Value == "" {
			return "dev"
		}
		return setting.Value
	}
	return "dev"
}
