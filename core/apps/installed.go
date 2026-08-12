package apps

import (
	"kyrux/core/settings"

	_ "kyrux/apps/meta"
)

func init() {
	settings.InstalledApps = []string{
		"meta",
	}
}
