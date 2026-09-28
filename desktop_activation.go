package main

import (
	"context"
	"time"
)

// Cria o worker antes do Wails, evitando corrida entre início e shutdown.
// stop cancela e drena inclusive uma chamada de apresentação já em andamento.
func startDesktopActivations(requests <-chan struct{}, activate func(context.Context)) (ready func(context.Context), stop func()) {
	lifetime, cancel := context.WithCancel(context.Background())
	windows := make(chan context.Context, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case <-lifetime.Done():
			return
		case window := <-windows:
			serveDesktopActivations(lifetime, window, requests, activate)
		}
	}()
	return func(window context.Context) {
			select {
			case <-lifetime.Done():
			case windows <- window:
			default:
			}
		}, func() {
			cancel()
			<-done
		}
}

// O callback IPC apenas enfileira. A janela só recebe pedidos após o startup
// publicar seus adapters, e nenhum serviço é reinicializado ao pedir foco.
func serveDesktopActivations(lifetime, window context.Context, requests <-chan struct{}, activate func(context.Context)) {
	// Preserva a apresentação inicial retardada do Windows, agora no mesmo
	// worker drenado no shutdown, em vez de uma goroutine independente.
	timer := time.NewTimer(400 * time.Millisecond)
	defer timer.Stop()
	initial := timer.C
	for {
		select {
		case <-lifetime.Done():
			return
		case <-window.Done():
			return
		case <-requests:
		case <-initial:
			initial = nil
		}
		if lifetime.Err() != nil || window.Err() != nil {
			return
		}
		activate(window)
	}
}
