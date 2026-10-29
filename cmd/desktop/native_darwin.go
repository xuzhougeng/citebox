//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
#include <stdlib.h>
#include <string.h>

static char *citebox_preferred_language(void) {
	@autoreleasepool {
		NSString *lang = [[NSLocale preferredLanguages] firstObject];
		if (lang == nil) {
			return NULL;
		}
		return strdup([lang UTF8String]);
	}
}

// mode: 0 follows the system, 1 is light, 2 is dark. Must run on the main thread.
static void citebox_set_window_appearance(void *window, int mode) {
	@autoreleasepool {
		NSWindow *nsWindow = (__bridge NSWindow *)window;
		if (nsWindow == nil) {
			return;
		}
		NSAppearance *appearance = nil;
		if (mode == 1) {
			appearance = [NSAppearance appearanceNamed:NSAppearanceNameAqua];
		} else if (mode == 2) {
			appearance = [NSAppearance appearanceNamed:NSAppearanceNameDarkAqua];
		}
		[nsWindow setAppearance:appearance];
	}
}
*/
import "C"

import (
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// systemLanguage returns the first preferred language of the macOS user,
// e.g. "zh-Hans-CN" or "en-US".
func systemLanguage() string {
	lang := C.citebox_preferred_language()
	if lang == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(lang))
	return C.GoString(lang)
}

// setWindowDarkAppearance makes the native window (title bar, traffic
// lights, scroll bars) match the app theme. Wails only applies
// MacWindow.Appearance at creation, so later theme changes go through here.
func setWindowDarkAppearance(window *application.WebviewWindow, dark bool) {
	if window == nil {
		return
	}
	mode := C.int(1)
	if dark {
		mode = 2
	}
	application.InvokeSync(func() {
		C.citebox_set_window_appearance(window.NativeWindow(), mode)
	})
}
