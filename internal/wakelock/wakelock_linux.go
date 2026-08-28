//go:build linux

package wakelock

import (
	"assistente/internal/logging"
	"context"

	"github.com/godbus/dbus/v5"
)

var cookie uint32

func enable() {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		logging.Warnf(context.Background(), "wakelock", "dbus session bus indisponível: %v", err)
		return
	}
	defer conn.Close() //nolint:errcheck
	obj := conn.Object("org.freedesktop.ScreenSaver", "/org/freedesktop/ScreenSaver")
	call := obj.Call("org.freedesktop.ScreenSaver.Inhibit", 0, "assistente", "janela em foco")
	if call.Err != nil {
		logging.Warnf(context.Background(), "wakelock", "Inhibit falhou: %v", call.Err)
		return
	}
	if err := call.Store(&cookie); err != nil {
		logging.Warnf(context.Background(), "wakelock", "Inhibit sem cookie: %v", err)
	}
}

func disable() {
	if cookie == 0 {
		return
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return
	}
	defer conn.Close() //nolint:errcheck
	obj := conn.Object("org.freedesktop.ScreenSaver", "/org/freedesktop/ScreenSaver")
	_ = obj.Call("org.freedesktop.ScreenSaver.UnInhibit", 0, cookie).Err
	cookie = 0
}
