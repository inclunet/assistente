$ErrorActionPreference = 'Stop'
$version = '3.11'
$expected = 'c7d27f780ddb6cffb4730138cd1591e841f4b7edb155856901cdf5f214394fa1'
$destRoot = Join-Path (Join-Path (Get-Location).Path 'build/bin/agent-build/tools') 'nsis'
$zipPath = Join-Path (Join-Path (Get-Location).Path 'build/bin/agent-build/tools') "nsis-$version.zip"
$installDir = Join-Path $destRoot "nsis-$version"
$urls = @(
  "https://downloads.sourceforge.net/project/nsis/NSIS%203/$version/nsis-$version.zip",
  "https://sourceforge.net/projects/nsis/files/NSIS%203/$version/nsis-$version.zip/download"
)

# Sempre instala a versão pinada: um makensis pré-existente no runner
# não entra no PATH à frente do que baixamos.
$downloaded = $false
foreach ($attempt in 1..5) {
  foreach ($url in $urls) {
    try {
      Write-Host "Baixando NSIS $version (tentativa $attempt): $url"
      New-Item -ItemType Directory -Force -Path (Split-Path $zipPath) | Out-Null
      Invoke-WebRequest -Uri $url -OutFile $zipPath -UserAgent 'Wget/1.21' -TimeoutSec 120
      if ((Get-Item -LiteralPath $zipPath).Length -le 1MB) {
        Write-Warning "Download incompleto ($((Get-Item -LiteralPath $zipPath).Length) bytes)"
        continue
      }
      $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $zipPath).Hash.ToLowerInvariant()
      if ($actual -ne $expected) {
        Write-Warning "SHA-256 divergente: got=$actual expected=$expected"
        Remove-Item -LiteralPath $zipPath -Force -ErrorAction SilentlyContinue
        continue
      }
      $downloaded = $true
      break
    } catch {
      Write-Warning $_.Exception.Message
    }
  }
  if ($downloaded) { break }
  Start-Sleep -Seconds (5 * $attempt)
}
if (-not $downloaded) {
  throw "Falha ao baixar NSIS $version com SHA-256 esperado ($expected)"
}

New-Item -ItemType Directory -Force -Path $destRoot | Out-Null

Expand-Archive -LiteralPath $zipPath -DestinationPath $destRoot -Force
$makensis = Join-Path $installDir 'makensis.exe'
if (-not (Test-Path -LiteralPath $makensis)) {
  throw "makensis.exe não encontrado em $installDir"
}
# GITHUB_PATH prepende nas steps seguintes; também no PATH desta step.
$installDir | Out-File -FilePath $env:GITHUB_PATH -Encoding utf8 -Append
$env:PATH = "$installDir$([IO.Path]::PathSeparator)$env:PATH"
& $makensis /VERSION
