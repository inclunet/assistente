package terminal

import (
	"fmt"
	"strings"
)

// A falha ao entrar no diretório impede a execução do comando do usuário.
// Aspas são escapadas segundo o shell, sem interpolação de caminhos.
func commandInDirectory(command, directory, shell string) string {
	if directory == "" {
		return command
	}
	if shell == "powershell" {
		path := strings.ReplaceAll(directory, "'", "''")
		return fmt.Sprintf("try { Microsoft.PowerShell.Management\\Set-Location -LiteralPath '%s' -ErrorAction Stop; %s } catch { Write-Error $_; $global:LASTEXITCODE = 1 }", path, command)
	}
	path := strings.ReplaceAll(directory, "'", "'\"'\"'")
	// eval recebe um único literal, sem expansão antecipada. Isso mantém a
	// leitura por linhas do shell, inclusive alias definido na linha anterior.
	script := strings.ReplaceAll(command, "'", "'\"'\"'")
	return fmt.Sprintf("if command cd -- '%s'; then command eval -- '%s'; else false; fi", path, script)
}
