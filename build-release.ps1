# Script de Release para GitHub Pages
# Gera binários e atualiza o manifest de updates

$VERSION = $args[0]
if (-not $VERSION) {
    Write-Host "Uso: .\build-release.ps1 <version>"
    Write-Host "Exemplo: .\build-release.ps1 1.0.1"
    exit 1
}

$RELEASE_DIR = "releases/$VERSION"
$MANIFEST_FILE = "update-manifest.json"

Write-Host "=== Gerando release v$VERSION ===" -ForegroundColor Cyan

# Define ldflags para injetar versão
$LDFLAGS = "-X main.AppVersion=$VERSION"

# Cria diretório de release
New-Item -ItemType Directory -Force -Path $RELEASE_DIR | Out-Null

# Plataformas suportadas
$platforms = @(
    @{os="windows"; arch="amd64"; ext=".exe"},
    @{os="darwin"; arch="amd64"; ext=""},
    @{os="darwin"; arch="arm64"; ext=""},
    @{os="linux"; arch="amd64"; ext=""}
)

$builds = @{}

foreach ($platform in $platforms) {
    $os = $platform.os
    $arch = $platform.arch
    $ext = $platform.ext
    $buildKey = "$os-$arch"
    $outputName = "assistente-$buildKey$ext"
    $outputPath = "$RELEASE_DIR/$outputName"
    
    Write-Host "`nBuildando para $buildKey..." -ForegroundColor Yellow
    
    # Configura variáveis de ambiente para cross-compilation
    $env:GOOS = $os
    $env:GOARCH = $arch
    $env:CGO_ENABLED = "1"
    
    # Build com Wails
    if ($os -eq "windows" -and $arch -eq "amd64") {
        # Build nativo Windows (pode usar CGO)
        wails build -platform windows/amd64 -clean -ldflags $LDFLAGS
        
        # Copia binário gerado
        $wailsOutput = "build\bin\assistente.exe"
        if (Test-Path $wailsOutput) {
            Copy-Item $wailsOutput $outputPath
        }
    } else {
        # Para outras plataformas, precisaria de toolchain específica
        # Por enquanto, documenta o processo manual
        Write-Host "  Nota: Build para $buildKey requer toolchain específica" -ForegroundColor Gray
        Write-Host "  Comando: wails build -platform $os/$arch -clean" -ForegroundColor Gray
        continue
    }
    
    if (Test-Path $outputPath) {
        # Calcula checksum SHA256
        $hash = (Get-FileHash -Path $outputPath -Algorithm SHA256).Hash.ToLower()
        $size = (Get-Item $outputPath).Length
        
        Write-Host "  ✓ Build concluído: $outputPath" -ForegroundColor Green
        Write-Host "  Checksum: sha256:$hash" -ForegroundColor Gray
        Write-Host "  Tamanho: $size bytes" -ForegroundColor Gray
        
        # Adiciona ao manifest
        $builds[$buildKey] = @{
            url = "https://inclunet.github.io/assistente/$RELEASE_DIR/$outputName"
            checksum = "sha256:$hash"
            size = $size
        }
    } else {
        Write-Host "  ✗ Falha no build" -ForegroundColor Red
    }
}

# Gera manifest JSON
Write-Host "`n=== Gerando manifest ===" -ForegroundColor Cyan

$manifest = @{
    version = $VERSION
    released = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
    notes = "Release $VERSION"
    builds = $builds
}

$manifestJson = $manifest | ConvertTo-Json -Depth 10
$manifestJson | Out-File -FilePath $MANIFEST_FILE -Encoding UTF8

Write-Host "✓ Manifest gerado: $MANIFEST_FILE" -ForegroundColor Green
Write-Host "`nConteúdo do manifest:" -ForegroundColor Gray
Write-Host $manifestJson

Write-Host "`n=== Próximos passos ===" -ForegroundColor Cyan
Write-Host "1. Revise os binários em: $RELEASE_DIR"
Write-Host "2. Teste os builds em cada plataforma"
Write-Host "3. Commit e push para branch gh-pages:"
Write-Host "   git checkout gh-pages"
Write-Host "   git add $RELEASE_DIR $MANIFEST_FILE"
Write-Host "   git commit -m 'Release v$VERSION'"
Write-Host "   git push origin gh-pages"
Write-Host "4. Configure GitHub Pages para servir da branch gh-pages"

Write-Host "`nDOCUMENTAÇÃO:" -ForegroundColor Magenta
Write-Host "- URL do manifest: https://inclunet.github.io/assistente/$MANIFEST_FILE"
Write-Host "- Diretório de releases: https://inclunet.github.io/assistente/$RELEASE_DIR"
