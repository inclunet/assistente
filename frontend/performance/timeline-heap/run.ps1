$ErrorActionPreference = 'Stop'
$taskRoot = (Resolve-Path (Join-Path $PSScriptRoot '../../..')).Path
$taskBuild = Join-Path $taskRoot 'build/bin/agent-build'
'go-work', 'go-cache', 'scratch', 'results' | ForEach-Object {
    New-Item -ItemType Directory -Force -Path (Join-Path $taskBuild $_) | Out-Null
}
$env:GOTMPDIR = Join-Path $taskBuild 'go-work'
$env:GOCACHE = Join-Path $taskBuild 'go-cache'
$env:TEMP = $env:TMP = $env:TMPDIR = Join-Path $taskBuild 'scratch'
Push-Location $taskRoot
try {
    node frontend/node_modules/typescript/bin/tsc -p frontend/performance/timeline-heap/tsconfig.json --noEmit
    if ($LASTEXITCODE -ne 0) { throw 'TypeScript falhou' }
    node frontend/node_modules/vite/bin/vite.js build --config frontend/performance/timeline-heap/vite.config.ts
    if ($LASTEXITCODE -ne 0) { throw 'Build da fixture falhou' }
    # O loader Go do Wails remove WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS.
    # Cópia de compilação somente do harness: não altera dependência nem app.
    $wailsModule = go list -m -f '{{.Dir}}' github.com/wailsapp/wails/v2
    if ($LASTEXITCODE -ne 0) { throw 'Não foi possível localizar o Wails' }
    $instrumentedModule = Join-Path $taskBuild 'timeline-heap-wails'
    New-Item -ItemType Directory -Force -Path $instrumentedModule | Out-Null
    Get-ChildItem -LiteralPath $wailsModule | Copy-Item -Destination $instrumentedModule -Recurse -Force
    $frontendSource = Join-Path $instrumentedModule 'internal/frontend/desktop/windows/frontend.go'
    $source = Get-Content -LiteralPath $frontendSource -Raw
    $anchor = 'chromium.MessageCallback = f.processMessage'
    if (($source.Split($anchor)).Count -ne 2) { throw 'Wails mudou; revisar instrumentação CDP' }
    Set-ItemProperty -LiteralPath $frontendSource -Name IsReadOnly -Value $false
    $source.Replace($anchor, 'chromium.AdditionalBrowserArgs = append(chromium.AdditionalBrowserArgs, "--remote-debugging-port="+os.Getenv("TIMELINE_HEAP_CDP_PORT"))' + "`n`t" + $anchor) |
        Set-Content -LiteralPath $frontendSource -Encoding utf8
    $modFile = Join-Path $taskBuild 'timeline-heap.mod'
    Copy-Item go.mod $modFile -Force
    Copy-Item go.sum (Join-Path $taskBuild 'timeline-heap.sum') -Force
    go mod edit -modfile $modFile -replace "github.com/wailsapp/wails/v2=$instrumentedModule"
    if ($LASTEXITCODE -ne 0) { throw 'Instrumentação do host falhou' }
    go build -modfile $modFile -tags 'production,timeline_heap' -o build/bin/agent-build/timeline-heap.exe ./tests/performance/timelineheap
    if ($LASTEXITCODE -ne 0) { throw 'Build do host WebView falhou' }
    node frontend/performance/timeline-heap/measure.mjs
    if ($LASTEXITCODE -ne 0) { throw 'Medição WebView falhou' }
} finally {
    Pop-Location
}
