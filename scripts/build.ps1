param(
    [string]$GoBinary = 'go',
    [string]$OutputDirectory = 'dist',
    [switch]$Package,
    [switch]$RegenerateIcon
)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path $PSScriptRoot -Parent
$savedEnvironment = @{}
foreach ($name in @('GOOS','GOARCH','CGO_ENABLED','GOTOOLCHAIN')) {
    $savedEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
}
Push-Location $projectRoot
try {
    $env:GOOS = 'windows'
    $env:GOARCH = 'amd64'
    $env:CGO_ENABLED = '0'
    $env:GOTOOLCHAIN = 'local'
    $resourcePath = 'cmd/hitbox-map/icon_windows_amd64.syso'
    if ($RegenerateIcon -or !(Test-Path -LiteralPath $resourcePath)) {
        & $GoBinary run ./tools/icon -out assets/HitboxMap.ico
        if ($LASTEXITCODE -ne 0) { throw '图标生成失败。' }
        & $GoBinary run github.com/akavel/rsrc@v0.10.2 -arch amd64 -ico assets/HitboxMap.ico -o $resourcePath
        if ($LASTEXITCODE -ne 0) { throw '图标资源编译失败。' }
    }
    $output = [IO.Path]::GetFullPath($OutputDirectory, $projectRoot)
    New-Item -ItemType Directory -Force $output | Out-Null
    $executable = Join-Path $output 'HitboxMap.exe'
    & $GoBinary build -trimpath -ldflags '-s -w -H=windowsgui' -o $executable ./cmd/hitbox-map
    if ($LASTEXITCODE -ne 0) { throw 'Go 编译失败。' }
    if ($Package) {
        $stageRoot = [IO.Path]::GetFullPath((Join-Path $projectRoot '.qa'))
        $stage = Join-Path $stageRoot ('package-' + [guid]::NewGuid().ToString('N'))
        try {
            New-Item -ItemType Directory -Path (Join-Path $stage 'locales') -Force | Out-Null
            Copy-Item -LiteralPath $executable,'README.md','README.en.md','README.ja.md','CHANGELOG.md' -Destination $stage
            Copy-Item -LiteralPath 'docs' -Destination $stage -Recurse
            foreach ($locale in @('en', 'zh', 'ja')) {
                Copy-Item -LiteralPath "locales/$locale.json" -Destination (Join-Path $stage 'locales')
            }
            $zipPath = Join-Path $output 'HitboxMap-windows-amd64.zip'
            Compress-Archive -LiteralPath (Join-Path $stage 'HitboxMap.exe'),(Join-Path $stage 'README.md'),(Join-Path $stage 'README.en.md'),(Join-Path $stage 'README.ja.md'),(Join-Path $stage 'CHANGELOG.md'),(Join-Path $stage 'locales'),(Join-Path $stage 'docs') -DestinationPath $zipPath -Force
            $checksums = @(Get-FileHash -LiteralPath $executable,$zipPath -Algorithm SHA256 | ForEach-Object { $_.Hash.ToLowerInvariant() + '  ' + [IO.Path]::GetFileName($_.Path) })
            [IO.File]::WriteAllText((Join-Path $output 'SHA256SUMS.txt'), ($checksums -join "`n") + "`n")
        } finally {
            $resolvedStage = [IO.Path]::GetFullPath($stage)
            if (!$resolvedStage.StartsWith($stageRoot + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
                throw '打包暂存路径不在工作目录内。'
            }
            if (Test-Path -LiteralPath $resolvedStage) { Remove-Item -LiteralPath $resolvedStage -Recurse -Force }
        }
    }
    Get-Item -LiteralPath $executable | Select-Object FullName, Length
} finally {
    Pop-Location
    foreach ($name in $savedEnvironment.Keys) {
        [Environment]::SetEnvironmentVariable($name, $savedEnvironment[$name], 'Process')
    }
}
