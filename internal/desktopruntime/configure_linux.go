//go:build linux

package desktopruntime

/*
#cgo pkg-config: gtk+-3.0
#include <gtk/gtk.h>

static gboolean citebox_confirm_close(GtkWidget *widget, GdkEvent *event, gpointer user_data) {
	const char *title = "退出 CiteBox";
	const char *primary = "Linux 版本暂不支持关闭后驻留后台。";
	const char *secondary = "关闭当前窗口将直接退出程序。";

	GtkWidget *dialog = gtk_message_dialog_new(
		GTK_WINDOW(widget),
		GTK_DIALOG_MODAL | GTK_DIALOG_DESTROY_WITH_PARENT,
		GTK_MESSAGE_QUESTION,
		GTK_BUTTONS_NONE,
		"%s",
		primary
	);
	gtk_window_set_title(GTK_WINDOW(dialog), title);
	gtk_message_dialog_format_secondary_text(GTK_MESSAGE_DIALOG(dialog), "%s", secondary);
	gtk_dialog_add_button(GTK_DIALOG(dialog), "取消", GTK_RESPONSE_CANCEL);
	gtk_dialog_add_button(GTK_DIALOG(dialog), "退出", GTK_RESPONSE_ACCEPT);
	gtk_dialog_set_default_response(GTK_DIALOG(dialog), GTK_RESPONSE_CANCEL);

	gint response = gtk_dialog_run(GTK_DIALOG(dialog));
	gtk_widget_destroy(dialog);
	return response == GTK_RESPONSE_ACCEPT ? FALSE : TRUE;
}

static void citebox_install_close_confirm(GtkWindow *window, const char *app_name) {
	if (window == NULL) {
		return;
	}
	g_signal_connect(G_OBJECT(window), "delete-event", G_CALLBACK(citebox_confirm_close), NULL);
}

static void citebox_activate_window(GtkWindow *window) {
	if (window == NULL) {
		return;
	}
	gtk_window_deiconify(window);
	gtk_window_present(window);
}
*/
import "C"

import (
	"unsafe"

	webview "github.com/webview/webview_go"
	"github.com/xuzhougeng/citebox/internal/desktopicon"
)

func Configure(w webview.WebView, _ string, _ desktopicon.Assets, _ ClosePreferenceStore) error {
	if err := bindExternalOpener(w); err != nil {
		return err
	}
	if err := initDesktopBridge(w); err != nil {
		return err
	}

	C.citebox_install_close_confirm((*C.GtkWindow)(w.Window()), nil)
	return nil
}

func ActivateWindow(window unsafe.Pointer) error {
	C.citebox_activate_window((*C.GtkWindow)(window))
	return nil
}
