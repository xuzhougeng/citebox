//go:build darwin

package desktopruntime

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
#import <objc/runtime.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

extern int citeboxRequestClosePrompt(uintptr_t windowToken);

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

@interface CiteBoxStatusController : NSObject <NSWindowDelegate, NSMenuDelegate> {
@public
	NSWindow *_window;
	id _originalDelegate;
	NSStatusItem *_statusItem;
	NSMenu *_menu;
	NSString *_appName;
	NSString *_iconPath;
	BOOL _showingMenu;
}
- (instancetype)initWithWindow:(NSWindow *)window appName:(NSString *)appName iconPath:(NSString *)iconPath;
- (BOOL)ensureStatusItem;
- (void)openWindow:(id)sender;
- (void)quitApp:(id)sender;
- (void)statusItemClicked:(id)sender;
- (void)showStatusItemMenu;
- (void)showTrayUnavailableAlert;
- (NSModalResponse)promptCloseAction;
@end

@implementation CiteBoxStatusController

- (instancetype)initWithWindow:(NSWindow *)window appName:(NSString *)appName iconPath:(NSString *)iconPath {
	self = [super init];
	if (self == nil) {
		return nil;
	}

	_window = window;
	_originalDelegate = [window delegate];
	_appName = [appName copy];
	_iconPath = [iconPath copy];
	return self;
}

- (BOOL)respondsToSelector:(SEL)selector {
	return [super respondsToSelector:selector] || [_originalDelegate respondsToSelector:selector];
}

- (id)forwardingTargetForSelector:(SEL)selector {
	if ([_originalDelegate respondsToSelector:selector]) {
		return _originalDelegate;
	}
	return [super forwardingTargetForSelector:selector];
}

- (BOOL)ensureStatusItem {
	if (_statusItem != nil) {
		return YES;
	}

	BOOL hasIcon = _iconPath != nil && [_iconPath length] > 0;
	CGFloat itemLength = hasIcon ? NSSquareStatusItemLength : NSVariableStatusItemLength;
	_statusItem = [[NSStatusBar systemStatusBar] statusItemWithLength:itemLength];
	NSStatusBarButton *button = [_statusItem button];
	if (button == nil) {
		[self removeStatusItem];
		return NO;
	}

	if (hasIcon) {
		NSImage *icon = [[NSImage alloc] initWithContentsOfFile:_iconPath];
		if (icon != nil) {
			[icon setTemplate:NO];
			[button setImage:icon];
		} else {
			[button setTitle:_appName ?: @"CiteBox"];
		}
	} else {
		[button setTitle:_appName ?: @"CiteBox"];
	}

	_menu = [[NSMenu alloc] initWithTitle:_appName ?: @"CiteBox"];
	[_menu setDelegate:self];

	NSMenuItem *openItem = [[[NSMenuItem alloc]
		initWithTitle:[NSString stringWithFormat:@"Open %@", _appName ?: @"CiteBox"]
		       action:@selector(openWindow:)
		keyEquivalent:@""] autorelease];
	[openItem setTarget:self];
	[_menu addItem:openItem];

	[_menu addItem:[NSMenuItem separatorItem]];

	NSMenuItem *quitItem = [[[NSMenuItem alloc]
		initWithTitle:[NSString stringWithFormat:@"Quit %@", _appName ?: @"CiteBox"]
		       action:@selector(quitApp:)
		keyEquivalent:@""] autorelease];
	[quitItem setTarget:self];
	[_menu addItem:quitItem];

	[button setTarget:self];
	[button setAction:@selector(statusItemClicked:)];
	[button sendActionOn:NSEventMaskLeftMouseUp | NSEventMaskRightMouseUp];
	return YES;
}

- (void)removeStatusItem {
	if (_statusItem == nil) {
		return;
	}
	[[NSStatusBar systemStatusBar] removeStatusItem:_statusItem];
	_statusItem = nil;
	_menu = nil;
	_showingMenu = NO;
}

- (void)openWindow:(id)sender {
	[self removeStatusItem];
	citebox_focus_window(_window);
}

- (void)quitApp:(id)sender {
	[self removeStatusItem];
	[[NSApplication sharedApplication] terminate:nil];
}

- (void)showTrayUnavailableAlert {
	NSAlert *alert = [[[NSAlert alloc] init] autorelease];
	[alert setAlertStyle:NSAlertStyleWarning];
	[alert setMessageText:@"无法最小化到托盘"];
	[alert setInformativeText:@"当前无法创建状态栏图标，窗口将保持打开。"];
	[alert addButtonWithTitle:@"确定"];
	[alert runModal];
}

- (NSModalResponse)promptCloseAction {
	NSAlert *alert = [[[NSAlert alloc] init] autorelease];
	[alert setAlertStyle:NSAlertStyleInformational];
	[alert setMessageText:@"关闭 CiteBox"];
	[alert setInformativeText:@"选择关闭窗口时的操作。"];
	[alert addButtonWithTitle:@"最小化到托盘"];
	[alert addButtonWithTitle:@"退出"];
	[alert addButtonWithTitle:@"取消"];
	return [alert runModal];
}

- (void)showStatusItemMenu {
	if (_showingMenu || _statusItem == nil || _menu == nil) {
		return;
	}

	NSStatusBarButton *button = [_statusItem button];
	if (button == nil) {
		return;
	}

	_showingMenu = YES;
	[_statusItem setMenu:_menu];
	[button setTarget:nil];
	[button setAction:nil];
	[button performClick:nil];
}

- (void)menuDidClose:(NSMenu *)menu {
	_showingMenu = NO;

	if (_statusItem == nil) {
		return;
	}

	[_statusItem setMenu:nil];

	NSStatusBarButton *button = [_statusItem button];
	if (button == nil) {
		return;
	}

	[button setTarget:self];
	[button setAction:@selector(statusItemClicked:)];
}

- (void)statusItemClicked:(id)sender {
	NSEvent *event = [NSApp currentEvent];
	if (event != nil &&
		([event type] == NSEventTypeRightMouseUp ||
			([event type] == NSEventTypeLeftMouseUp &&
				([event modifierFlags] & NSEventModifierFlagControl) == NSEventModifierFlagControl))) {
		[self showStatusItemMenu];
		return;
	}
	[self openWindow:sender];
}

- (BOOL)windowShouldClose:(NSWindow *)sender {
	if (citeboxRequestClosePrompt((uintptr_t)sender)) {
		return NO;
	}

	NSModalResponse response = [self promptCloseAction];
	if (response == NSAlertFirstButtonReturn) {
		if ([self ensureStatusItem]) {
			[sender orderOut:nil];
		} else {
			[self showTrayUnavailableAlert];
		}
		return NO;
	}
	if (response == NSAlertSecondButtonReturn) {
		[self quitApp:nil];
		return NO;
	}
	return NO;
}

@end

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

static const void *citebox_status_controller_key = &citebox_status_controller_key;

static CiteBoxStatusController *citebox_status_controller_for_window(NSWindow *window) {
	if (window == nil) {
		return nil;
	}
	return objc_getAssociatedObject(window, citebox_status_controller_key);
}

static const char *citebox_install_status_item(void *windowPtr, const char *appNameCString, const char *iconPathCString) {
	@autoreleasepool {
		NSWindow *window = (NSWindow *)windowPtr;
		if (window == nil) {
			return "missing window";
		}

		CiteBoxStatusController *controller = objc_getAssociatedObject(window, citebox_status_controller_key);
		if (controller != nil) {
			return NULL;
		}

		NSString *appName = appNameCString != NULL ? [NSString stringWithUTF8String:appNameCString] : @"CiteBox";
		NSString *iconPath = nil;
		if (iconPathCString != NULL && iconPathCString[0] != '\0') {
			iconPath = [NSString stringWithUTF8String:iconPathCString];
		}

		controller = [[[CiteBoxStatusController alloc] initWithWindow:window appName:appName iconPath:iconPath] autorelease];
		objc_setAssociatedObject(window, citebox_status_controller_key, controller, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
		[window setDelegate:controller];
		return NULL;
	}
}

static const char *citebox_minimize_to_status_item(void *windowPtr) {
	@autoreleasepool {
		NSWindow *window = (NSWindow *)windowPtr;
		CiteBoxStatusController *controller = citebox_status_controller_for_window(window);
		if (controller == nil) {
			return "missing status controller";
		}
		if (![controller ensureStatusItem]) {
			return "Failed to create status item";
		}
		[window orderOut:nil];
		return NULL;
	}
}

static void citebox_exit_desktop_app(void *windowPtr) {
	@autoreleasepool {
		NSWindow *window = (NSWindow *)windowPtr;
		CiteBoxStatusController *controller = citebox_status_controller_for_window(window);
		if (controller != nil) {
			[controller quitApp:nil];
			return;
		}
		[[NSApplication sharedApplication] terminate:nil];
	}
}

static const char *citebox_activate_window(void *windowPtr) {
	@autoreleasepool {
		NSWindow *window = (NSWindow *)windowPtr;
		if (window == nil) {
			return "missing window";
		}

		CiteBoxStatusController *controller = citebox_status_controller_for_window(window);
		if (controller != nil) {
			[controller openWindow:nil];
			return NULL;
		}

		citebox_focus_window(window);
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
	"github.com/xuzhougeng/citebox/internal/desktopicon"
)

func Configure(w webview.WebView, appName string, iconAssets desktopicon.Assets, closePreferenceStore ClosePreferenceStore) error {
	if err := bindExternalOpener(w); err != nil {
		return err
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
	if err := bindClosePromptActions(w, func() error {
		return minimizeToTray(w.Window())
	}, func() error {
		return exitDesktopApp(w.Window())
	}, closePreferenceStore); err != nil {
		return err
	}

	cAppName := C.CString(appName)
	defer C.free(unsafe.Pointer(cAppName))
	cIconPath := C.CString(iconAssets.PNGPath)
	defer C.free(unsafe.Pointer(cIconPath))

	C.citebox_install_app_menu(cAppName)
	if errMessage := C.citebox_install_status_item(w.Window(), cAppName, cIconPath); errMessage != nil {
		return fmt.Errorf("install macOS status item integration: %s", C.GoString(errMessage))
	}
	C.citebox_focus_window(w.Window())
	return nil
}

func minimizeToTray(window unsafe.Pointer) error {
	if errMessage := C.citebox_minimize_to_status_item(window); errMessage != nil {
		return fmt.Errorf("minimize to status item: %s", C.GoString(errMessage))
	}
	return nil
}

func exitDesktopApp(window unsafe.Pointer) error {
	C.citebox_exit_desktop_app(window)
	return nil
}

func ActivateWindow(window unsafe.Pointer) error {
	if errMessage := C.citebox_activate_window(window); errMessage != nil {
		return fmt.Errorf("activate window: %s", C.GoString(errMessage))
	}
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
