//go:build !windows

package httpapi

import (
	"fmt"

	"21loader-cross/internal/core"
)

func triggerLocalAppUpdate(_ string) (core.AppUpdateResponse, error) {
	return core.AppUpdateResponse{}, fmt.Errorf("mise a jour locale indisponible sur cette plateforme")
}
