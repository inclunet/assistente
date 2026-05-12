# Instalador do Assistente CLI (asst) para Windows
# Uso: irm https://raw.githubusercontent.com/inclunet/assistente/main/install.ps1 | iex
#
# Variáveis opcionais:
#   $env:INSTALL_DIR  - diretório de instalação (padrão: ~\.local\bin)
#   $env:VERSION      - versão específica (padrão: latest)

$ErrorActionPreference = 'Stop'

$Repo = "inclunet/assistente"
$Binary = "asst"

function Get-LatestVersion {
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -UseBasicParsing
    return $release.tag_name
}

function Get-InstallDir {
    if ($env:INSTALL_DIR) { return $env:INSTALL_DIR }

    $dir = Join-Path $HOME ".local\bin"
    if (-not (Test-Path $dir)) {
        New-Item -ItemType Directory -Path $dir -Force | Out-Null
    }
    return $dir
}

function Add-ToPath {
    param([string]$Dir)

    $currentPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($currentPath -split ';' -contains $Dir) { return }

    [Environment]::SetEnvironmentVariable("Path", "$Dir;$currentPath", "User")
    $env:Path = "$Dir;$env:Path"
    Write-Host "  Adicionado $Dir ao PATH do usuario." -ForegroundColor Yellow
}

function Main {
    Write-Host "Instalando $Binary para windows/amd64..." -ForegroundColor Cyan

    # Versao
    if ($env:VERSION) {
        $tag = $env:VERSION
    } else {
        $tag = Get-LatestVersion
    }

    if (-not $tag) {
        Write-Host "Erro: nao foi possivel determinar a versao." -ForegroundColor Red
        exit 1
    }

    Write-Host "  Versao: $tag" -ForegroundColor Green

    # Download
    $assetName = "$Binary-windows-amd64.exe"
    $url = "https://github.com/$Repo/releases/download/$tag/$assetName"
    $tmpFile = Join-Path $env:TEMP "$Binary.exe"

    Write-Host "  Baixando $url..."
    try {
        Invoke-WebRequest -Uri $url -OutFile $tmpFile -UseBasicParsing
    } catch {
        Write-Host "Erro no download. Verifique se a versao $tag existe." -ForegroundColor Red
        exit 1
    }
    Write-Host "  Download OK" -ForegroundColor Green

    # Checksum
    $checksumUrl = "$url.sha256"
    $checksumFile = Join-Path $env:TEMP "checksum.sha256"
    try {
        Invoke-WebRequest -Uri $checksumUrl -OutFile $checksumFile -UseBasicParsing
        $expected = (Get-Content $checksumFile -Raw).Trim().Split(' ')[0]
        $actual = (Get-FileHash $tmpFile -Algorithm SHA256).Hash.ToLower()
        if ($expected -ne $actual) {
            Write-Host "Checksum invalido! Esperado: $expected, obtido: $actual" -ForegroundColor Red
            exit 1
        }
        Write-Host "  Checksum verificado" -ForegroundColor Green
    } catch {
        Write-Host "  Checksum nao disponivel (continuando)" -ForegroundColor Yellow
    }

    # Instalar
    $destDir = Get-InstallDir
    $dest = Join-Path $destDir "$Binary.exe"

    Move-Item -Path $tmpFile -Destination $dest -Force
    Write-Host "  Instalado em $dest" -ForegroundColor Green

    # Adicionar ao PATH
    Add-ToPath $destDir

    # Verificar
    Write-Host ""
    & $dest version
    Write-Host ""
    Write-Host "Uso:" -ForegroundColor Cyan
    Write-Host "  $Binary setup          # Configuracao inicial"
    Write-Host "  $Binary chat `"ola`"     # Conversar"
    Write-Host "  $Binary --help         # Ajuda completa"
}

Main
