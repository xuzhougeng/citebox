//go:build darwin

package desktopruntime

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
#include <stdlib.h>
#include <string.h>

static NSView *citebox_find_first_responder_view(NSView *view) {
	if (view == nil) {
		return nil;
	}
	if ([view acceptsFirstResponder]) {
		return view;
	}
	for (NSView *subview in [view subviews]) {
		NSView *candidate = citebox_find_first_responder_view(subview);
		if (candidate != nil) {
			return candidate;
		}
	}
	return nil;
}

static NSMenuItem *citebox_new_menu_item(NSString *title, SEL action, NSString *keyEquivalent, NSEventModifierFlags modifiers) {
	NSMenuItem *item = [[NSMenuItem alloc] initWithTitle:title action:action keyEquivalent:keyEquivalent ?: @""];
	[item setKeyEquivalentModifierMask:modifiers];
	return item;
}

static void citebox_install_app_menu(const char *appNameCString) {
	@autoreleasepool {
		NSApplication *app = [NSApplication sharedApplication];
		if (app == nil) {
			return;
		}

		NSMenu *mainMenu = [app mainMenu];
		if (mainMenu == nil) {
			mainMenu = [[NSMenu alloc] initWithTitle:@""];
			[app setMainMenu:mainMenu];
		}

		if ([mainMenu numberOfItems] > 0) {
			return;
		}

		NSString *appName = appNameCString ? [NSString stringWithUTF8String:appNameCString] : @"App";

		NSMenuItem *appMenuItem = [[NSMenuItem alloc] initWithTitle:@"" action:nil keyEquivalent:@""];
		[mainMenu addItem:appMenuItem];

		NSMenu *appMenu = [[NSMenu alloc] initWithTitle:appName];
		[appMenuItem setSubmenu:appMenu];
		[appMenu addItem:citebox_new_menu_item([@"Hide " stringByAppendingString:appName], @selector(hide:), @"h", NSEventModifierFlagCommand)];
		[appMenu addItem:citebox_new_menu_item(@"Hide Others", @selector(hideOtherApplications:), @"h", NSEventModifierFlagCommand | NSEventModifierFlagOption)];
		[appMenu addItem:citebox_new_menu_item(@"Show All", @selector(unhideAllApplications:), @"", 0)];
		[appMenu addItem:[NSMenuItem separatorItem]];
		[appMenu addItem:citebox_new_menu_item([@"Quit " stringByAppendingString:appName], @selector(terminate:), @"q", NSEventModifierFlagCommand)];

		NSMenuItem *editMenuItem = [[NSMenuItem alloc] initWithTitle:@"" action:nil keyEquivalent:@""];
		[mainMenu addItem:editMenuItem];

		NSMenu *editMenu = [[NSMenu alloc] initWithTitle:@"Edit"];
		[editMenuItem setSubmenu:editMenu];
		[editMenu addItem:citebox_new_menu_item(@"Undo", @selector(undo:), @"z", NSEventModifierFlagCommand)];
		[editMenu addItem:citebox_new_menu_item(@"Redo", @selector(redo:), @"Z", NSEventModifierFlagCommand | NSEventModifierFlagShift)];
		[editMenu addItem:[NSMenuItem separatorItem]];
		[editMenu addItem:citebox_new_menu_item(@"Cut", @selector(cut:), @"x", NSEventModifierFlagCommand)];
		[editMenu addItem:citebox_new_menu_item(@"Copy", @selector(copy:), @"c", NSEventModifierFlagCommand)];
		[editMenu addItem:citebox_new_menu_item(@"Paste", @selector(paste:), @"v", NSEventModifierFlagCommand)];
		[editMenu addItem:citebox_new_menu_item(@"Select All", @selector(selectAll:), @"a", NSEventModifierFlagCommand)];
	}
}

static void citebox_focus_window(void *windowPtr) {
	@autoreleasepool {
		NSWindow *window = (NSWindow *)windowPtr;
		if (window == nil) {
			return;
		}

		NSApplication *app = [NSApplication sharedApplication];
		if (app != nil) {
			[app activateIgnoringOtherApps:YES];
		}

		[window makeKeyAndOrderFront:nil];

		NSView *contentView = [window contentView];
		NSView *target = citebox_find_first_responder_view(contentView);
		if (target != nil) {
			[window makeFirstResponder:target];
		}
	}
}

static const char *citebox_open_external(const char *urlCString) {
	@autoreleasepool {
		if (urlCString == NULL) {
			return "missing url";
		}

		NSString *urlString = [NSString stringWithUTF8String:urlCString];
		if (urlString == nil || [urlString length] == 0) {
			return "invalid url";
		}

		NSURL *url = [NSURL URLWithString:urlString];
		if (url == nil) {
			return "invalid url";
		}

		if (![[NSWorkspace sharedWorkspace] openURL:url]) {
			return "failed to open url";
		}

		return NULL;
	}
}

static const char *citebox_save_file(const char *filenameCString, const void *bytes, int length, int *didSave) {
	@autoreleasepool {
		if (didSave != NULL) {
			*didSave = 0;
		}
		if (filenameCString == NULL) {
			return "missing filename";
		}
		if (length < 0) {
			return "invalid file length";
		}
		if (length > 0 && bytes == NULL) {
			return "missing file data";
		}

		NSString *filename = [NSString stringWithUTF8String:filenameCString];
		if (filename == nil || [filename length] == 0) {
			filename = @"download";
		}

		NSSavePanel *panel = [NSSavePanel savePanel];
		[panel setCanCreateDirectories:YES];
		[panel setNameFieldStringValue:filename];

		if ([panel runModal] != NSModalResponseOK) {
			return NULL;
		}

		NSData *data = length > 0
			? [NSData dataWithBytes:bytes length:(NSUInteger)length]
			: [NSData data];
		NSURL *targetURL = [panel URL];
		if (targetURL == nil) {
			return "missing target url";
		}

		NSError *error = nil;
		if (![data writeToURL:targetURL options:NSDataWritingAtomic error:&error]) {
			return "failed to save file";
		}

		if (didSave != NULL) {
			*didSave = 1;
		}
		return NULL;
	}
}

static char *citebox_read_clipboard_text(void) {
	@autoreleasepool {
		NSPasteboard *pasteboard = [NSPasteboard generalPasteboard];
		if (pasteboard == nil) {
			return NULL;
		}

		NSString *text = [pasteboard stringForType:NSPasteboardTypeString];
		if (text == nil) {
			return NULL;
		}

		const char *utf8 = [text UTF8String];
		if (utf8 == NULL) {
			return NULL;
		}

		return strdup(utf8);
	}
}

static const char *citebox_write_clipboard_text(const char *textCString) {
	@autoreleasepool {
		if (textCString == NULL) {
			return "missing text";
		}

		NSPasteboard *pasteboard = [NSPasteboard generalPasteboard];
		if (pasteboard == nil) {
			return "missing pasteboard";
		}

		NSString *text = [NSString stringWithUTF8String:textCString];
		if (text == nil) {
			text = @"";
		}

		[pasteboard clearContents];
		if (![pasteboard setString:text forType:NSPasteboardTypeString]) {
			return "failed to write clipboard";
		}

		return NULL;
	}
}
*/
import "C"

import (
	"encoding/base64"
	"fmt"
	"unsafe"

	webview "github.com/webview/webview_go"
)

func Configure(w webview.WebView, appName string) error {
	if err := w.Bind("citeboxDesktopOpenExternal", func(url string) error {
		return openExternal(url)
	}); err != nil {
		return fmt.Errorf("bind external opener: %w", err)
	}
	if err := w.Bind("citeboxDesktopReadClipboardText", func() (string, error) {
		return readClipboardText()
	}); err != nil {
		return fmt.Errorf("bind clipboard reader: %w", err)
	}
	if err := w.Bind("citeboxDesktopWriteClipboardText", func(text string) error {
		return writeClipboardText(text)
	}); err != nil {
		return fmt.Errorf("bind clipboard writer: %w", err)
	}
	if err := w.Bind("citeboxDesktopSaveFile", func(filename string, dataBase64 string) (map[string]bool, error) {
		saved, err := saveFile(filename, dataBase64)
		if err != nil {
			return nil, err
		}
		return map[string]bool{"saved": saved}, nil
	}); err != nil {
		return fmt.Errorf("bind file saver: %w", err)
	}

	if err := initDesktopBridge(w); err != nil {
		return err
	}

	cAppName := C.CString(appName)
	defer C.free(unsafe.Pointer(cAppName))

	C.citebox_install_app_menu(cAppName)
	C.citebox_focus_window(w.Window())
	return nil
}

func openExternal(url string) error {
	cURL := C.CString(url)
	defer C.free(unsafe.Pointer(cURL))

	if errMessage := C.citebox_open_external(cURL); errMessage != nil {
		return fmt.Errorf("open external url: %s", C.GoString(errMessage))
	}
	return nil
}

func saveFile(filename string, dataBase64 string) (bool, error) {
	data, err := base64.StdEncoding.DecodeString(dataBase64)
	if err != nil {
		return false, fmt.Errorf("decode file data: %w", err)
	}

	cFilename := C.CString(filename)
	defer C.free(unsafe.Pointer(cFilename))

	var payload unsafe.Pointer
	if len(data) > 0 {
		payload = C.CBytes(data)
		defer C.free(payload)
	}

	var didSave C.int
	if errMessage := C.citebox_save_file(cFilename, payload, C.int(len(data)), &didSave); errMessage != nil {
		return false, fmt.Errorf("save file: %s", C.GoString(errMessage))
	}

	return didSave != 0, nil
}

func readClipboardText() (string, error) {
	value := C.citebox_read_clipboard_text()
	if value == nil {
		return "", nil
	}
	defer C.free(unsafe.Pointer(value))
	return C.GoString(value), nil
}

func writeClipboardText(text string) error {
	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))

	if errMessage := C.citebox_write_clipboard_text(cText); errMessage != nil {
		return fmt.Errorf("write clipboard: %s", C.GoString(errMessage))
	}
	return nil
}
