//go:build linux

package main

/*
#cgo pkg-config: gtk+-3.0
#include <gtk/gtk.h>
#include <stdlib.h>

static void showFatalErrorGTK(const char *title, const char *message) {
	int argc = 0;
	char **argv = NULL;
	if (!gtk_init_check(&argc, &argv)) {
		return;
	}
	GtkWidget *dialog = gtk_message_dialog_new(
		NULL,
		GTK_DIALOG_MODAL,
		GTK_MESSAGE_ERROR,
		GTK_BUTTONS_OK,
		"%s",
		message
	);
	gtk_window_set_title(GTK_WINDOW(dialog), title);
	gtk_dialog_run(GTK_DIALOG(dialog));
	gtk_widget_destroy(dialog);
	while (gtk_events_pending()) {
		gtk_main_iteration();
	}
}
*/
import "C"

import (
	"runtime"
	"unsafe"
)

var showNativeFatalError = showFatalErrorGTK

func showFatalErrorGTK(title, message string) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	cTitle := C.CString(title)
	defer C.free(unsafe.Pointer(cTitle))
	cMessage := C.CString(message)
	defer C.free(unsafe.Pointer(cMessage))
	C.showFatalErrorGTK(cTitle, cMessage)
}
