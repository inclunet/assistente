package terminal

import (
	"encoding/base64"
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
		path := base64.StdEncoding.EncodeToString([]byte(directory))
		// Go entrega caminho canônico. Um reparse point novo, inclusive num
		// ancestral, significa que o startup substituiu a origem aprovada.
		// As APIs usadas existem também no Windows PowerShell 5.
		guard := "for ($__asstDirectory = [System.IO.DirectoryInfo] $__asstPath; $null -ne $__asstDirectory; $__asstDirectory = $__asstDirectory.Parent) { if (([System.IO.File]::GetAttributes($__asstDirectory.FullName) -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) { throw 'Approved directory changed' } }; "
		return fmt.Sprintf("try { $__asstPath = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('%s')); %s Microsoft.PowerShell.Management\\Set-Location -LiteralPath $__asstPath -ErrorAction Stop; %s %s"+powershellDirectoryCatch, path, guard, guard, command)
	}
	path := strings.ReplaceAll(directory, "'", "'\"'\"'")
	// eval recebe um único literal, sem expansão antecipada. Isso mantém a
	// leitura por linhas do shell, inclusive alias definido na linha anterior.
	// Espaço inicial evita opções de eval sem depender de "--", que dash
	// interpreta como nome de comando em vez de delimitador de opções.
	script := " " + strings.ReplaceAll(command, "'", "'\"'\"'")
	return fmt.Sprintf("if command cd -P -- '%s' && command test \"$(command pwd -P)\" = '%s'; then command eval '%s'; else false; fi", path, path, script)
}
