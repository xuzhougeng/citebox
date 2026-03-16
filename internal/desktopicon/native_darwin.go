//go:build darwin

package desktopicon

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework AppKit
#import <Cocoa/Cocoa.h>
#include <stdlib.h>

static const char *citebox_set_app_icon(const char *path) {
	@autoreleasepool {
		NSString *iconPath = [NSString stringWithUTF8String:path];
		if (iconPath == nil) {
			return "invalid icon path";
		}

		NSImage *icon = [[NSImage alloc] initWithContentsOfFile:iconPath];
		if (icon == nil) {
			return "failed to load icon image";
		}

		[[NSApplication sharedApplication] setApplicationIconImage:icon];
		return NULL;
	}
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

func applyNativeWindowIcon(_ unsafe.Pointer, iconPath string) error {
	if iconPath == "" {
		return nil
	}

	cPath := C.CString(iconPath)
	defer C.free(unsafe.Pointer(cPath))

	if errMessage := C.citebox_set_app_icon(cPath); errMessage != nil {
		return fmt.Errorf("set macOS app icon: %s", C.GoString(errMessage))
	}

	return nil
}
