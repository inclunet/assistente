package terminal

import (
	"fmt"
	"strings"
)

const powershellDirectoryCatch = " } catch { $global:LASTEXITCODE = 1; Microsoft.PowerShell.Utility\\Write-Error -ErrorRecord $_ -ErrorAction Continue }"

// A falha ao entrar no diretório impede a execução do comando do usuário.
// Aspas são escapadas segundo o shell, sem interpolação de caminhos.
func commandInDirectory(command, directory, shell string) string {
	if directory == "" {
		return command
	}
	if shell == "powershell" {
		path := strings.ReplaceAll(directory, "'", "''")
		// Go entrega caminho canônico. Um reparse point novo, inclusive num
		// ancestral, significa que o startup substituiu a origem aprovada.
		// As APIs usadas existem também no Windows PowerShell 5.
		guard := fmt.Sprintf("for ($__asstDirectory = [System.IO.DirectoryInfo] '%s'; $null -ne $__asstDirectory; $__asstDirectory = $__asstDirectory.Parent) { if (([System.IO.File]::GetAttributes($__asstDirectory.FullName) -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) { throw 'Approved directory changed' } }; ", path)
		return fmt.Sprintf("try { %s Microsoft.PowerShell.Management\\Set-Location -LiteralPath '%s' -ErrorAction Stop; %s %s"+powershellDirectoryCatch, guard, path, guard, command)
	}
	path := strings.ReplaceAll(directory, "'", "'\"'\"'")
	// eval recebe um único literal, sem expansão antecipada. Isso mantém a
	// leitura por linhas do shell, inclusive alias definido na linha anterior.
	// Espaço inicial evita opções de eval sem depender de "--", que dash
	// interpreta como nome de comando em vez de delimitador de opções.
	script := " " + strings.ReplaceAll(command, "'", "'\"'\"'")
	return fmt.Sprintf("if command cd -P -- '%s' && command test \"$(command pwd -P)\" = '%s'; then command eval '%s'; else false; fi", path, path, script)
}
