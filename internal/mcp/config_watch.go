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
	watchDebounce     = 500 * time.Millisecond
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
	defer func() { _ = watcher.Close() }()

	if err := m.resolver.EnsureHomeDir(); err != nil {
		log.Printf("[MCP:watch] Erro ao garantir diretório home: %v", err)
	}

	dirs := m.resolver.GetSearchPaths()
	watchedDirs := make(map[string]struct{})
	watching := m.addConfigWatchDirs(watcher, dirs, watchedDirs)

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
			if ev.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) == 0 {
				continue
			}
			if !shouldHandleConfigWatchEvent(ev.Name, dirs) {
				continue
			}

			watching += m.addConfigWatchDirs(watcher, dirs, watchedDirs)

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

type configWatcher interface {
	Add(string) error
}

func (m *Manager) addConfigWatchDirs(watcher configWatcher, dirs []string, watchedDirs map[string]struct{}) int {
	added := 0

	for _, dir := range dirs {
		if addWatchDir(watcher, dir, watchedDirs) {
			added++
			continue
		}

		// Se o diretório mcp ainda não existe, observa o ancestral existente
		// mais próximo para detectar quando .assistente/mcp for criado por fora.
		for parent := filepath.Dir(dir); parent != "." && parent != dir; parent = filepath.Dir(parent) {
			if _, ok := watchedDirs[filepath.Clean(parent)]; ok {
				break
			}
			if addWatchDir(watcher, parent, watchedDirs) {
				added++
				break
			}
		}
	}

	return added
}

func addWatchDir(watcher configWatcher, dir string, watchedDirs map[string]struct{}) bool {
	cleanDir := filepath.Clean(dir)
	if _, ok := watchedDirs[cleanDir]; ok {
		return false
	}

	if info, err := os.Stat(cleanDir); err != nil || !info.IsDir() {
		return false
	}

	if err := watcher.Add(cleanDir); err != nil {
		log.Printf("[MCP:watch] Erro ao observar %s: %v", cleanDir, err)
		return false
	}

	watchedDirs[cleanDir] = struct{}{}
	log.Printf("[MCP:watch] Observando %s", cleanDir)
	return true
}

func shouldHandleConfigWatchEvent(path string, dirs []string) bool {
	cleanPath := filepath.Clean(path)

	for _, dir := range dirs {
		cleanDir := filepath.Clean(dir)
		if isConfigFile(cleanPath) && isPathWithin(cleanPath, cleanDir) {
			return true
		}
		if cleanPath == cleanDir {
			return true
		}
		if isPathWithin(cleanDir, cleanPath) {
			return true
		}
	}

	return false
}

func isPathWithin(path, parent string) bool {
	rel, err := filepath.Rel(parent, path)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
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
		m.applyInlineAuthFromConfig(f.Name, &cfg, data)

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
			go func() { _ = m.Disconnect(slug) }()
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
