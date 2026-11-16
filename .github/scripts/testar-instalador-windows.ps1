param([Parameter(Mandatory = $true)][string]$Makensis)
$ErrorActionPreference = 'Stop'

# Compila um pacote de teste, sem executar nem instalar nada na máquina.
$repo = (Get-Location).Path
$fixture = Join-Path $repo 'build/bin/agent-build/nsis-smoke'
$scriptDir = Join-Path $fixture 'windows/installer'
@($scriptDir, (Join-Path $scriptDir 'tmp'), (Join-Path $fixture 'bin')) | ForEach-Object {
    New-Item -ItemType Directory -Force -Path $_ | Out-Null
}
Copy-Item -LiteralPath (Join-Path $repo 'build/windows/installer/project.nsi') -Destination $scriptDir
Copy-Item -LiteralPath (Join-Path $repo 'build/windows/installer/wails_tools.nsh') -Destination $scriptDir
Copy-Item -LiteralPath (Join-Path $repo 'build/windows/icon.ico') -Destination (Join-Path $fixture 'windows/icon.ico')
Copy-Item -LiteralPath (Join-Path $repo 'build/bin/asst-amd64.exe') -Destination (Join-Path $fixture 'bin/asst-amd64.exe')
[IO.File]::WriteAllBytes((Join-Path $fixture 'bin/desktop-fixture.exe'), [byte[]](0, 1))
[IO.File]::WriteAllBytes((Join-Path $scriptDir 'tmp/MicrosoftEdgeWebview2Setup.exe'), [byte[]](0, 1))
& $Makensis /V2 "/DARG_WAILS_AMD64_BINARY=$(Join-Path $fixture 'bin/desktop-fixture.exe')" (Join-Path $scriptDir 'project.nsi')
if ($LASTEXITCODE -ne 0) { throw "NSIS failed: $LASTEXITCODE" }
if (-not (Test-Path -LiteralPath (Join-Path $fixture 'bin/assistente-amd64-installer.exe'))) {
    throw 'NSIS did not produce the expected test installer'
}
