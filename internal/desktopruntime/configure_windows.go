//go:build windows

package desktopruntime

/*
#define UNICODE
#define _UNICODE
#include <windows.h>
#include <shellapi.h>
#include <stdlib.h>

#define CITEBOX_TRAY_CALLBACK_MESSAGE (WM_APP + 1)
#define CITEBOX_TRAY_OPEN_COMMAND 1001
#define CITEBOX_TRAY_EXIT_COMMAND 1002

static HWND citebox_tray_hwnd = NULL;
static WNDPROC citebox_prev_wndproc = NULL;
static NOTIFYICONDATAW citebox_tray_icon;
static HMENU citebox_tray_menu = NULL;
static HICON citebox_tray_hicon = NULL;
static BOOL citebox_tray_icon_owned = FALSE;
static BOOL citebox_tray_visible = FALSE;

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

static void citebox_remove_tray_icon(void) {
	if (citebox_tray_visible) {
		Shell_NotifyIconW(NIM_DELETE, &citebox_tray_icon);
		citebox_tray_visible = FALSE;
	}
}

static void citebox_cleanup_tray(void) {
	citebox_remove_tray_icon();
	ZeroMemory(&citebox_tray_icon, sizeof(citebox_tray_icon));

	if (citebox_tray_menu != NULL) {
		DestroyMenu(citebox_tray_menu);
		citebox_tray_menu = NULL;
	}
	if (citebox_tray_icon_owned && citebox_tray_hicon != NULL) {
		DestroyIcon(citebox_tray_hicon);
	}
	citebox_tray_hicon = NULL;
	citebox_tray_icon_owned = FALSE;
}

static void citebox_restore_window(HWND hwnd) {
	citebox_remove_tray_icon();
	if (IsIconic(hwnd)) {
		ShowWindow(hwnd, SW_RESTORE);
	} else {
		ShowWindow(hwnd, SW_SHOW);
	}
	SetForegroundWindow(hwnd);
}

static void citebox_show_tray_menu(HWND hwnd) {
	if (citebox_tray_menu == NULL) {
		return;
	}

	POINT cursor;
	GetCursorPos(&cursor);
	SetForegroundWindow(hwnd);
	TrackPopupMenu(citebox_tray_menu, TPM_RIGHTBUTTON, cursor.x, cursor.y, 0, hwnd, NULL);
	PostMessageW(hwnd, WM_NULL, 0, 0);
}

static BOOL citebox_ensure_tray_icon(HWND hwnd) {
	if (citebox_tray_visible) {
		return TRUE;
	}

	citebox_tray_icon.hWnd = hwnd;
	if (!Shell_NotifyIconW(NIM_ADD, &citebox_tray_icon)) {
		return FALSE;
	}

	citebox_tray_visible = TRUE;
	Shell_NotifyIconW(NIM_SETVERSION, &citebox_tray_icon);
	return TRUE;
}

static LRESULT CALLBACK citebox_window_proc(HWND hwnd, UINT msg, WPARAM wp, LPARAM lp) {
	switch (msg) {
	case WM_CLOSE:
		if (citebox_ensure_tray_icon(hwnd)) {
			ShowWindow(hwnd, SW_HIDE);
			return 0;
		}
		break;
	case WM_COMMAND:
		switch (LOWORD(wp)) {
		case CITEBOX_TRAY_OPEN_COMMAND:
			citebox_restore_window(hwnd);
			return 0;
		case CITEBOX_TRAY_EXIT_COMMAND:
			citebox_cleanup_tray();
			DestroyWindow(hwnd);
			return 0;
		}
		break;
	case CITEBOX_TRAY_CALLBACK_MESSAGE:
		switch ((UINT)lp) {
		case WM_LBUTTONUP:
		case WM_LBUTTONDBLCLK:
			citebox_restore_window(hwnd);
			return 0;
		case WM_RBUTTONUP:
		case WM_CONTEXTMENU:
			citebox_show_tray_menu(hwnd);
			return 0;
		}
		break;
	case WM_DESTROY:
		citebox_cleanup_tray();
		if (citebox_prev_wndproc != NULL) {
			SetWindowLongPtrW(hwnd, GWLP_WNDPROC, (LONG_PTR)citebox_prev_wndproc);
		}
		citebox_prev_wndproc = NULL;
		citebox_tray_hwnd = NULL;
		break;
	}

	if (citebox_prev_wndproc != NULL) {
		return CallWindowProcW(citebox_prev_wndproc, hwnd, msg, wp, lp);
	}
	return DefWindowProcW(hwnd, msg, wp, lp);
}

static const char *citebox_install_tray(HWND hwnd, const char *app_name, const char *icon_path) {
	if (hwnd == NULL) {
		return "missing window handle";
	}
	if (citebox_tray_hwnd == hwnd && citebox_prev_wndproc != NULL) {
		return NULL;
	}

	citebox_cleanup_tray();

	citebox_tray_menu = CreatePopupMenu();
	if (citebox_tray_menu == NULL) {
		return "CreatePopupMenu failed";
	}
	AppendMenuW(citebox_tray_menu, MF_STRING, CITEBOX_TRAY_OPEN_COMMAND, L"Open CiteBox");
	AppendMenuW(citebox_tray_menu, MF_STRING, CITEBOX_TRAY_EXIT_COMMAND, L"Exit");

	if (icon_path != NULL && icon_path[0] != '\0') {
		wchar_t *wide_icon_path = citebox_utf8_to_utf16(icon_path);
		if (wide_icon_path != NULL) {
			citebox_tray_hicon = (HICON)LoadImageW(
				NULL,
				wide_icon_path,
				IMAGE_ICON,
				GetSystemMetrics(SM_CXSMICON),
				GetSystemMetrics(SM_CYSMICON),
				LR_LOADFROMFILE
			);
			free(wide_icon_path);
			if (citebox_tray_hicon != NULL) {
				citebox_tray_icon_owned = TRUE;
			}
		}
	}
	if (citebox_tray_hicon == NULL) {
		citebox_tray_hicon = LoadIconW(NULL, IDI_APPLICATION);
		citebox_tray_icon_owned = FALSE;
	}

	ZeroMemory(&citebox_tray_icon, sizeof(citebox_tray_icon));
	citebox_tray_icon.cbSize = sizeof(citebox_tray_icon);
	citebox_tray_icon.hWnd = hwnd;
	citebox_tray_icon.uID = 1;
	citebox_tray_icon.uFlags = NIF_MESSAGE | NIF_ICON | NIF_TIP;
	citebox_tray_icon.uCallbackMessage = CITEBOX_TRAY_CALLBACK_MESSAGE;
	citebox_tray_icon.hIcon = citebox_tray_hicon;
	citebox_tray_icon.uVersion = NOTIFYICON_VERSION_4;

	wchar_t *wide_app_name = citebox_utf8_to_utf16(app_name != NULL ? app_name : "CiteBox");
	if (wide_app_name != NULL) {
		lstrcpynW(citebox_tray_icon.szTip, wide_app_name, sizeof(citebox_tray_icon.szTip) / sizeof(WCHAR));
		free(wide_app_name);
	}

	citebox_prev_wndproc = (WNDPROC)SetWindowLongPtrW(hwnd, GWLP_WNDPROC, (LONG_PTR)citebox_window_proc);
	if (citebox_prev_wndproc == NULL) {
		citebox_cleanup_tray();
		return "SetWindowLongPtrW failed";
	}

	citebox_tray_hwnd = hwnd;
	return NULL;
}
*/
import "C"

import (
	"fmt"
	"unsafe"

	webview "github.com/webview/webview_go"
	"github.com/xuzhougeng/citebox/internal/desktopicon"
)

func Configure(w webview.WebView, appName string, iconAssets desktopicon.Assets) error {
	if err := bindExternalOpener(w); err != nil {
		return err
	}
	if err := initDesktopBridge(w); err != nil {
		return err
	}

	cAppName := C.CString(appName)
	defer C.free(unsafe.Pointer(cAppName))

	cIconPath := C.CString(iconAssets.ICOPath)
	defer C.free(unsafe.Pointer(cIconPath))

	if errMessage := C.citebox_install_tray((C.HWND)(w.Window()), cAppName, cIconPath); errMessage != nil {
		return fmt.Errorf("install windows tray integration: %s", C.GoString(errMessage))
	}
	return nil
}
