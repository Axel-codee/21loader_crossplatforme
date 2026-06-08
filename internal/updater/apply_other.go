//go:build !windows && !darwin

package updater

import (
	"context"
	"fmt"
	"runtime"
)

func ApplyPackage(_ context.Context, _ string) (string, error) {
	return "", fmt.Errorf("mise a jour automatique indisponible sur %s/%s: aucun package Linux n'est encore publie", runtime.GOOS, runtime.GOARCH)
}
