package runtime

import (
	"os"
	"strings"
)

const EnvExperimentalBackgroundRuntime = "LINKCLAW_EXPERIMENTAL_BACKGROUND_RUNTIME"

func BackgroundRuntimeEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvExperimentalBackgroundRuntime))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func RuntimeMode() string {
	if BackgroundRuntimeEnabled() {
		return "background-experimental"
	}
	return "host-managed"
}
