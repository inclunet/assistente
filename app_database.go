package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"assistente/internal/config"
	"assistente/internal/database"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ResetDatabase apaga o banco de dados, resetando ao estado inicial
func (a *App) ResetDatabase() error {
	configPath, err := config.GetConfigPath()
	if err != nil {
		return fmt.Errorf("erro ao obter caminho do banco de dados: %v", err)
	}

	dbPath := filepath.Join(filepath.Dir(configPath), "conversations.db")

	// Fecha a conexão com o banco de dados antes de deletar
	if err := database.Close(); err != nil {
		return fmt.Errorf("erro ao fechar banco de dados: %v", err)
	}

	// Aguarda um momento para garantir que o arquivo foi liberado
	time.Sleep(100 * time.Millisecond)

	// Verifica se o arquivo existe
	if _, err := os.Stat(dbPath); err == nil {
		// Remove o banco de dados
		if err := os.Remove(dbPath); err != nil {
			return fmt.Errorf("erro ao remover banco de dados: %v", err)
		}

		// Remove arquivos auxiliares do SQLite (WAL e SHM)
		os.Remove(dbPath + "-wal")
		os.Remove(dbPath + "-shm")
	}

	// Reinicializa o banco de dados
	if err := database.Init(); err != nil {
		return fmt.Errorf("erro ao reinicializar banco: %v", err)
	}

	log.Println("[ResetDatabase] Banco resetado com sucesso")

	// Emite evento para o frontend limpar o estado
	runtime.EventsEmit(a.ctx, "database:reset")

	return nil
}

// ClearMessages apaga todas as mensagens e conversas, mantendo a estrutura do banco
func (a *App) ClearMessages() error {
	if err := database.ClearAllConversations(); err != nil {
		return fmt.Errorf("erro ao limpar mensagens e conversas: %v", err)
	}

	log.Println("[ClearMessages] Mensagens e conversas apagadas")
	runtime.EventsEmit(a.ctx, "messages:cleared")

	return nil
}
