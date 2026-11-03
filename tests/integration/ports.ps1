param([string]$Executable = 'dist/HitboxMap.exe')
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
Add-Type -Path (Join-Path $PSScriptRoot '../helpers/TrayProbe.cs')
$exePath = [IO.Path]::GetFullPath($Executable, $projectRoot)
$runDirectory = Join-Path $projectRoot ('.qa/ports-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $runDirectory -Force | Out-Null
$owned = [Collections.Generic.List[Diagnostics.Process]]::new()
$blockers = [Collections.Generic.List[Net.Sockets.TcpListener]]::new()

function Start-TestInstance([string]$name) {
    $directory = Join-Path $runDirectory $name
    $instance = Start-Process -FilePath $exePath -ArgumentList '-data-dir', ('"' + $directory + '"'), '-no-browser' -WindowStyle Hidden -PassThru
    $owned.Add($instance)
    return $instance
}
function Wait-Ready([Diagnostics.Process]$instance, [int]$port) {
    for ($attempt = 0; $attempt -lt 80; $attempt++) {
        if ($instance.HasExited) { throw "Instance exited before listening on $port" }
        try {
            $health = Invoke-RestMethod "http://127.0.0.1:$port/api/health" -TimeoutSec 1
            $window = [TrayProbe]::Find($instance.Id)
            if ($health.app -eq 'hitbox-map' -and $health.protocol -eq 2 -and $window -ne [IntPtr]::Zero -and [TrayProbe]::HasIcon($window)) {
                $owners = @(Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction Stop)
                if ($instance.Id -notin $owners.OwningProcess) { throw 'Port belongs to a different process' }
                return
            }
        } catch { }
        Start-Sleep -Milliseconds 100
    }
    throw "Instance did not start on $port"
}
function Stop-TestInstance([Diagnostics.Process]$instance) {
    if (!$instance.HasExited) {
        [TrayProbe]::Exit($instance.Id)
        if (!$instance.WaitForExit(5000)) { throw 'Shutdown timed out' }
    }
}
try {
    foreach ($port in 29301..29303) {
        $check = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, $port)
        $check.Server.ExclusiveAddressUse = $true
        $check.Start()
        $check.Stop()
    }
    $first = Start-TestInstance 'default'
    Wait-Ready $first 29301
    $duplicate = Start-TestInstance 'default-duplicate'
    if (!$duplicate.WaitForExit(5000) -or $duplicate.ExitCode -ne 0) { throw 'Default duplicate did not exit successfully' }
    if (Test-Path (Join-Path $runDirectory 'default-duplicate')) { throw 'Default duplicate created data' }
    if ($first.HasExited) { throw 'Existing instance was stopped' }
    Stop-TestInstance $first

    foreach ($port in 29301..29302) {
        $blocker = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, $port)
        $blocker.Server.ExclusiveAddressUse = $true
        $blocker.Start()
        $blockers.Add($blocker)
    }
    $fallback = Start-TestInstance 'fallback'
    Wait-Ready $fallback 29303
    $fallbackDuplicate = Start-TestInstance 'fallback-duplicate'
    if (!$fallbackDuplicate.WaitForExit(6000) -or $fallbackDuplicate.ExitCode -ne 0) { throw 'Fallback duplicate did not exit successfully' }
    if (Test-Path (Join-Path $runDirectory 'fallback-duplicate')) { throw 'Fallback duplicate created data' }
    if ($fallback.HasExited) { throw 'Fallback instance was stopped' }
    $result = [ordered]@{defaultPort=29301;defaultReuseExitCode=$duplicate.ExitCode;foreignPorts=@(29301,29302);fallbackPort=29303;fallbackReuseExitCode=$fallbackDuplicate.ExitCode;duplicateDataCreated=$false;trayReady=$true}
    $result | ConvertTo-Json | Set-Content (Join-Path $runDirectory 'results.json')
    $result | ConvertTo-Json -Compress
} finally {
    foreach ($instance in $owned) {
        if (!$instance.HasExited) {
            try { Stop-TestInstance $instance } catch { $instance.Kill(); $instance.WaitForExit() }
        }
    }
    foreach ($blocker in $blockers) { $blocker.Stop() }
}
