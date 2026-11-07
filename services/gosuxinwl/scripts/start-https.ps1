param([switch]$Restart)

$ErrorActionPreference = 'Stop'
$projectRoot = (Resolve-Path -LiteralPath (Split-Path -Parent $PSScriptRoot)).Path
$binaryPath = Join-Path $projectRoot 'bin/caddy.exe'
$configPath = Join-Path $projectRoot 'manifest/https/Caddyfile'
if (-not (Test-Path -LiteralPath $binaryPath -PathType Leaf)) { throw 'Install the official Caddy binary at bin/caddy.exe first.' }
if (-not (Test-Path -LiteralPath $configPath -PathType Leaf)) { throw 'HTTPS configuration is missing: manifest/https/Caddyfile.' }
$binaryPath = (Resolve-Path -LiteralPath $binaryPath).Path
$runtimePath = Join-Path $projectRoot 'runtime'
New-Item -ItemType Directory -Path $runtimePath -Force | Out-Null
$requiredPorts = @(443, 9880)
$pidPath = Join-Path $runtimePath 'https-proxy.pid'
$certificateStatus = 'Certificate issuance/renewal status is recorded in runtime/https-stderr.log.'
if ((Get-Content -LiteralPath $configPath -Raw) -match 'bootstrap-cert\.pem') {
    $certificateStatus = 'Temporary self-signed certificate is active; a publicly trusted certificate has not been issued.'
}

function Get-HTTPSProxyListeners {
    return @(Get-NetTCPConnection -LocalPort $requiredPorts -State Listen -ErrorAction SilentlyContinue)
}

function Assert-HTTPSProxyOwners($Listeners) {
    $ownerIds = @($Listeners | Select-Object -ExpandProperty OwningProcess -Unique)
    foreach ($ownerId in $ownerIds) {
        $owner = Get-Process -Id $ownerId -ErrorAction Stop
        if ($owner.Path -ne $binaryPath) {
            throw "HTTPS port 443 or 9880 belongs to a different application (PID $ownerId); nothing was stopped."
        }
    }
    if ($ownerIds.Count -gt 1) { throw 'HTTPS ports 443 and 9880 belong to different processes; resolve the conflicting instances first.' }
}

function Test-HTTPSProxyReady($Listeners, [int]$OwnerProcessId) {
    foreach ($port in $requiredPorts) {
        $matches = @($Listeners | Where-Object { $_.LocalPort -eq $port -and $_.OwningProcess -eq $OwnerProcessId -and $_.LocalAddress -in @('0.0.0.0', '::') })
        if ($matches.Count -eq 0) { return $false }
        # Windows can report an IPv4-capable Go listener as dual-stack ::.
        $probe = [System.Net.Sockets.TcpClient]::new([System.Net.Sockets.AddressFamily]::InterNetwork)
        try {
            $pending = $probe.ConnectAsync('127.0.0.1', $port)
            if (-not $pending.Wait(1000) -or -not $probe.Connected) { return $false }
        } catch {
            return $false
        } finally {
            $probe.Dispose()
        }
    }
    return $true
}

# Validate before stopping any running process. Validation does not start a listener.
$quotedConfig = '"' + $configPath + '"'
$validation = Start-Process -FilePath $binaryPath -ArgumentList @('validate', '--config', $quotedConfig, '--adapter', 'caddyfile') -WorkingDirectory $projectRoot -WindowStyle Hidden -RedirectStandardOutput (Join-Path $runtimePath 'https-validate-stdout.log') -RedirectStandardError (Join-Path $runtimePath 'https-validate-stderr.log') -PassThru
# Windows PowerShell 5.1 can lose ExitCode after a redirected Start-Process
# exits unless its process handle is retained before waiting.
$validationHandle = $validation.Handle
if (-not $validation.WaitForExit(30000)) {
    $validation.Refresh()
    if (-not $validation.HasExited) { Stop-Process -InputObject $validation -ErrorAction Stop }
    throw 'Caddy configuration validation timed out; inspect runtime/https-validate-stderr.log.'
}
$validation.WaitForExit()
if ($validation.ExitCode -ne 0) { throw 'Caddy configuration validation failed; inspect runtime/https-validate-stderr.log.' }

$listeners = @(Get-HTTPSProxyListeners)
Assert-HTTPSProxyOwners $listeners
$ownedProcesses = @(Get-Process -Name 'caddy' -ErrorAction SilentlyContinue | Where-Object { $_.Path -eq $binaryPath })
if ($ownedProcesses.Count -gt 1) { throw 'Multiple Caddy processes use this project binary; resolve the conflicting instances first.' }
if ($ownedProcesses.Count -eq 1) {
    $runningProcess = $ownedProcesses[0]
    if ($Restart) {
        $confirmedProcess = Get-Process -Id $runningProcess.Id -ErrorAction Stop
        if ($confirmedProcess.Path -ne $binaryPath -or $confirmedProcess.StartTime -ne $runningProcess.StartTime) { throw 'The HTTPS process changed; restart was canceled.' }
        Stop-Process -InputObject $confirmedProcess -ErrorAction Stop
        if (-not $confirmedProcess.WaitForExit(10000)) { throw 'The project HTTPS process did not exit within 10 seconds.' }
        if (@(Get-HTTPSProxyListeners).Count -gt 0) { throw 'HTTPS port 443 or 9880 is still occupied; startup was canceled.' }
    } else {
        if (-not (Test-HTTPSProxyReady $listeners $runningProcess.Id)) { throw 'The project HTTPS process is running without both public listeners. Run .\scripts\start-https.ps1 -Restart and inspect runtime/https-stderr.log.' }
        $runningProcess.Id | Set-Content -LiteralPath $pidPath -Encoding ASCII
        Write-Output "HTTPS proxy is listening on 0.0.0.0:443 and 0.0.0.0:9880 (PID $($runningProcess.Id)); public URL=https://kf.szputy.com:9880/suxinweb/. $certificateStatus"
        return
    }
} elseif ($listeners.Count -gt 0) {
    throw 'The HTTPS listener process changed during inspection; startup was canceled.'
}

# Recheck just before launch so an unrelated listener is never stopped or replaced.
if (@(Get-HTTPSProxyListeners).Count -gt 0) { throw 'HTTPS port 443 or 9880 became occupied; startup was canceled.' }
$httpsProcess = Start-Process -FilePath $binaryPath -ArgumentList @('run', '--config', $quotedConfig, '--adapter', 'caddyfile') -WorkingDirectory $projectRoot -WindowStyle Hidden -RedirectStandardOutput (Join-Path $runtimePath 'https-stdout.log') -RedirectStandardError (Join-Path $runtimePath 'https-stderr.log') -PassThru
$httpsProcess.Id | Set-Content -LiteralPath $pidPath -Encoding ASCII
$deadline = [DateTime]::UtcNow.AddSeconds(30)
while ([DateTime]::UtcNow -lt $deadline) {
    $httpsProcess.Refresh()
    if ($httpsProcess.HasExited) { break }
    $listeners = @(Get-HTTPSProxyListeners)
    Assert-HTTPSProxyOwners $listeners
    if (Test-HTTPSProxyReady $listeners $httpsProcess.Id) {
        Write-Output "HTTPS proxy started on 0.0.0.0:443 and 0.0.0.0:9880 (PID $($httpsProcess.Id)); public URL=https://kf.szputy.com:9880/suxinweb/. $certificateStatus"
        return
    }
    Start-Sleep -Milliseconds 250
}
throw 'HTTPS proxy startup did not complete; inspect runtime/https-stdout.log and runtime/https-stderr.log.'
