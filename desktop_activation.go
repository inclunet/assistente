package main

import "context"

// O callback IPC apenas enfileira. A janela só recebe pedidos após o startup
// publicar seus adapters, e nenhum serviço é reinicializado ao pedir foco.
func serveDesktopActivations(lifetime, window context.Context, requests <-chan struct{}, activate func(context.Context)) {
	for {
		select {
		case <-lifetime.Done():
			return
		case <-window.Done():
			return
		case <-requests:
			if lifetime.Err() != nil || window.Err() != nil {
				return
			}
			activate(window)
		}
	}
}
