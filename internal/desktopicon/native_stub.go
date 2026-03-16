//go:build !linux && !darwin && !windows

package desktopicon

import "unsafe"

func applyNativeWindowIcon(_ unsafe.Pointer, _ string) error {
	return nil
}
