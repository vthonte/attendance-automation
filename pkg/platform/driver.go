package platform

import (
	"attendance/pkg/core"
)

func GetDriver() core.PlatformDriver {
	return newPlatformDriver()
}
