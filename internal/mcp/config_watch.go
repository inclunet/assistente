package mcp

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

const (
	watchDebounce    = 500 * time.Millisecond
	selfWriteCooldown = 2 * time.Second
)

// WatchConfigs inicia observação dos diretórios de config MCP.
// Detecta criação, modificação e remoção de arquivos .json feitas
// fora do app (editor de texto, git pull, etc.) e sincroniza o estado.
// Bloqueia até m.ctx ser cancelado (chamar em goroutine).
func (m *Manager) WatchConfigs() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("[MCP:watch] Erro ao criar watcher: %v", err)
		return
	}
	defer watcher.Close()

	if err := m.resolver.EnsureHomeDir(); err != nil {
		log.Printf("[MCP:watch] Erro ao garantir diretório home: %v", err)
	}

	dirs := m.resolver.GetSearchPaths()
	watching := 0
	for _, dir := range dirs {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			if err := watcher.Add(dir); err != nil {
				log.Printf("[MCP:watch] Erro ao observar %s: %v", dir, err)
			} else {
				watching++
				log.Printf("[MCP:watch] Observando %s", dir)
			}
		}
	}

	if watching == 0 {
		log.Printf("[MCP:watch] Nenhum diretório de config encontrado para observar")
		return
	}

	debounce := time.NewTimer(watchDebounce)
	debounce.Stop()
	pending := false

	for {
		select {
		case <-m.ctx.Done():
			return

		case ev, ok := <-watcher.Events:
			if !ok {
				return
			}
			if !isConfigFile(ev.Name) {
				continue
			}
			if ev.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) == 0 {
				continue
			}

			log.Printf("[MCP:watch] Arquivo alterado: %s (%s)", filepath.Base(ev.Name), ev.Op)
			pending = true
			debounce.Reset(watchDebounce)

		case <-debounce.C:
			if !pending {
				continue
			}
			pending = false

			m.mu.RLock()
			recentSelfWrite := time.Since(m.lastSelfWrite) < selfWriteCooldown
			m.mu.RUnlock()

			if recentSelfWrite {
				log.Printf("[MCP:watch] Ignorando — escrita recente do próprio app")
				continue
			}

			m.syncConfigsFromDisk()

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("[MCP:watch] Erro do watcher: %v", err)
		}
	}
}

// syncConfigsFromDisk relê todos os configs do disco e sincroniza com o estado em memória.
func (m *Manager) syncConfigsFromDisk() {
	files, err := m.resolver.List()
	if err != nil {
		log.Printf("[MCP:watch] Erro ao listar configs: %v", err)
		return
	}

	onDisk := make(map[string]ServerConfig)

	for _, f := range files {
		if !strings.HasSuffix(f.Filename, configExt) {
			continue
		}

		data, _, err := m.resolver.Read(f.Filename)
		if err != nil {
			log.Printf("[MCP:watch] Erro ao ler %s: %v", f.Filename, err)
			continue
		}

		cfg, err := ParseServerConfig(data, f.Name)
		if err != nil {
			log.Printf("[MCP:watch] Erro ao parsear %s: %v", f.Filename, err)
			continue
		}

		onDisk[f.Name] = cfg
	}

	changed := false
	var autoConnectSlugs []string

	m.mu.Lock()

	// Detecta novos e modificados
	for slug, cfg := range onDisk {
		existing, ok := m.servers[slug]
		if !ok {
			m.servers[slug] = &ServerStatus{
				Slug:   slug,
				Config: cfg,
				Status: StatusDisconnected,
				Tools:  []MCPToolInfo{},
			}
			log.Printf("[MCP:watch] Novo servidor detectado: %s", slug)
			changed = true

			if cfg.Enabled && cfg.AutoConnect {
				autoConnectSlugs = append(autoConnectSlugs, slug)
			}
			continue
		}

		if configChanged(existing.Config, cfg) {
			existing.Config = cfg
			log.Printf("[MCP:watch] Config atualizado: %s", slug)
			changed = true
		}
	}

	// Detecta removidos
	for slug := range m.servers {
		if _, ok := onDisk[slug]; !ok {
			log.Printf("[MCP:watch] Servidor removido: %s — desconectando", slug)
			changed = true
			go m.Disconnect(slug)
			delete(m.servers, slug)
		}
	}

	m.mu.Unlock()

	if changed {
		m.emit("mcp:config_changed", nil)
	}

	for _, slug := range autoConnectSlugs {
		log.Printf("[MCP:watch] Auto-conectando novo servidor: %s", slug)
		go func(s string) {
			if err := m.Connect(s); err != nil {
				log.Printf("[MCP:watch] Erro ao auto-conectar '%s': %v", s, err)
			}
		}(slug)
	}
}

func isConfigFile(path string) bool {
	base := filepath.Base(path)
	return strings.HasSuffix(base, configExt) &&
		!strings.HasPrefix(base, ".") &&
		!strings.HasPrefix(base, "~")
}

// configChanged faz comparação rápida por JSON serializado.
func configChanged(a, b ServerConfig) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) != string(jb)
}
