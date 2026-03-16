//go:build linux

package desktopicon

/*
#cgo pkg-config: gtk+-3.0
#include <gtk/gtk.h>
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"unsafe"
)

func applyNativeWindowIcon(window unsafe.Pointer, iconPath string) error {
	if window == nil || iconPath == "" {
		return nil
	}

	cPath := C.CString(iconPath)
	defer C.free(unsafe.Pointer(cPath))

	var gErr *C.GError
	ok := C.gtk_window_set_icon_from_file((*C.GtkWindow)(window), cPath, &gErr)
	if ok != 0 {
		return nil
	}

	if gErr != nil {
		message := C.GoString((*C.char)(gErr.message))
		C.g_error_free(gErr)
		return fmt.Errorf("set linux window icon: %s", message)
	}

	return fmt.Errorf("set linux window icon failed")
}
