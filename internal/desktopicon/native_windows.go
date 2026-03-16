//go:build windows

package desktopicon

/*
#define UNICODE
#define _UNICODE
#include <windows.h>
#include <stdlib.h>

static wchar_t *citebox_utf8_to_utf16(const char *src) {
	int len = MultiByteToWideChar(CP_UTF8, 0, src, -1, NULL, 0);
	if (len <= 0) {
		return NULL;
	}

	wchar_t *dst = (wchar_t *)malloc(sizeof(wchar_t) * len);
	if (dst == NULL) {
		return NULL;
	}

	if (MultiByteToWideChar(CP_UTF8, 0, src, -1, dst, len) <= 0) {
		free(dst);
		return NULL;
	}

	return dst;
}

static const char *citebox_set_window_icon(HWND hwnd, const char *path) {
	wchar_t *widePath = citebox_utf8_to_utf16(path);
	if (widePath == NULL) {
		return "failed to convert icon path to UTF-16";
	}

	HICON icon = (HICON)LoadImageW(NULL, widePath, IMAGE_ICON, 0, 0, LR_LOADFROMFILE | LR_DEFAULTSIZE);
	free(widePath);
	if (icon == NULL) {
		return "LoadImageW failed";
	}

	SendMessageW(hwnd, WM_SETICON, ICON_BIG, (LPARAM)icon);
	SendMessageW(hwnd, WM_SETICON, ICON_SMALL, (LPARAM)icon);
	return NULL;
}
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

	if errMessage := C.citebox_set_window_icon((C.HWND)(window), cPath); errMessage != nil {
		return fmt.Errorf("set windows app icon: %s", C.GoString(errMessage))
	}

	return nil
}
