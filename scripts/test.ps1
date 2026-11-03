param([string]$GoBinary = 'go')
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path $PSScriptRoot -Parent
Push-Location $projectRoot
try {
    & $GoBinary test ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go tests failed.' }
    & $GoBinary vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go vet failed.' }
    $nodeTests = @(Get-ChildItem -LiteralPath 'tests/frontend','tests/scripts' -File -Filter '*.test.mjs' | Select-Object -ExpandProperty FullName)
    & node --test @nodeTests
    if ($LASTEXITCODE -ne 0) { throw 'Node tests failed.' }
} finally {
    Pop-Location
}
