package terminal

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// O escopo filho preserva a localização do runspace e descarta as variáveis da guarda.
func powerShellDirectoryGuard(directory string) string {
	if directory == "" {
		return ""
	}
	path := base64.StdEncoding.EncodeToString([]byte(directory))
	guard := "for ($__asstDirectory = [System.IO.DirectoryInfo] $__asstPath; $null -ne $__asstDirectory; $__asstDirectory = $__asstDirectory.Parent) { if (([System.IO.File]::GetAttributes($__asstDirectory.FullName) -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) { throw 'Approved directory changed' } }; "
	return fmt.Sprintf("& { $__asstPath = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('%s')); %s Microsoft.PowerShell.Management\\Set-Location -LiteralPath $__asstPath -ErrorAction Stop; %s }; ", path, guard, guard)
}

// A falha ao entrar no diretório impede a execução do comando do usuário.
// Aspas são escapadas segundo o shell, sem interpolação de caminhos.
func posixCommandInDirectory(command, directory string) string {
	if directory == "" {
		return command
	}
	path := strings.ReplaceAll(directory, "'", "'\"'\"'")
	// eval recebe um único literal, sem expansão antecipada. Isso mantém a
	// leitura por linhas do shell, inclusive alias definido na linha anterior.
	return fmt.Sprintf("if command cd -P -- '%s' && command test \"$(command pwd -P)\" = '%s'; then command eval %s; else false; fi", path, path, quotePOSIXScript(command))
}
