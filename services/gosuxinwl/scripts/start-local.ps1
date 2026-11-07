param([switch]$Restart)

$ErrorActionPreference = 'Stop'
$projectRoot = (Resolve-Path -LiteralPath (Split-Path -Parent $PSScriptRoot)).Path
$binaryPath = (Resolve-Path -LiteralPath (Join-Path $projectRoot 'bin/gosuxin.exe')).Path
$configPath = Join-Path $projectRoot 'manifest/config/config.yaml'
if (-not (Test-Path -LiteralPath $configPath)) {
    throw 'Initialize and configure manifest/config/config.yaml first.'
}
New-Item -ItemType Directory -Path (Join-Path $projectRoot 'runtime') -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $projectRoot 'resource/uploads') -Force | Out-Null
$configText = Get-Content -LiteralPath $configPath -Raw
$addressMatch = [regex]::Match($configText, '(?m)^server:\s*\r?\n[ \t]+address:\s*["'']?([^"''\r\n]+)')
if (-not $addressMatch.Success) { throw 'server.address is missing from local config.' }
$listenAddress = $addressMatch.Groups[1].Value.Trim()
$endpointMatch = [regex]::Match($listenAddress, '^(?<host>\[[^\]]+\]|[^:]*):(?<port>[0-9]+)$')
if (-not $endpointMatch.Success) { throw 'Invalid server.address; use an IP address and port.' }
$listenHost = $endpointMatch.Groups['host'].Value.Trim([char[]]'[]')
if ($listenHost -eq '') { $listenHost = '0.0.0.0' }
$listenPort = 0
if (-not [int]::TryParse($endpointMatch.Groups['port'].Value, [ref]$listenPort) -or $listenPort -lt 1 -or $listenPort -gt 65535) { throw 'Invalid server.address port.' }
function Test-ConfiguredListenerAddress([string]$LocalAddress, [string]$ExpectedHost = $listenHost, [int]$Port = $listenPort) {
    $allowedAddresses = @($ExpectedHost)
    if ($ExpectedHost -eq 'localhost') { $allowedAddresses = @('127.0.0.1', '::1') }
    if ($allowedAddresses -contains $LocalAddress) { return $true }
    # Windows may report Go's IPv4 wildcard listener as an IPv6 dual-stack
    # socket. Only accept it after confirming that IPv4 actually connects.
    if ($ExpectedHost -ne '0.0.0.0' -or $LocalAddress -ne '::') { return $false }
    $probe = [System.Net.Sockets.TcpClient]::new([System.Net.Sockets.AddressFamily]::InterNetwork)
    try {
        $pending = $probe.ConnectAsync('127.0.0.1', $Port)
        if (-not $pending.Wait(1000)) { return $false }
        return $probe.Connected
    } catch {
        return $false
    } finally {
        $probe.Dispose()
    }
}
$httpsHost = ''
$httpsPort = 0
$httpsSummary = ''
$httpsMatch = [regex]::Match($configText, '(?m)^[ \t]+httpsAddr:[ \t]*["'']?([^"''\r\n]+)')
if ($httpsMatch.Success -and $httpsMatch.Groups[1].Value.Trim() -ne '') {
    $httpsEndpoint = [regex]::Match($httpsMatch.Groups[1].Value.Trim(), '^(?<host>\[[^\]]+\]|[^:]*):(?<port>[0-9]+)$')
    if (-not $httpsEndpoint.Success) { throw 'Invalid server.httpsAddr.' }
    $httpsHost = $httpsEndpoint.Groups['host'].Value.Trim([char[]]'[]')
    if ($httpsHost -eq '') { $httpsHost = '0.0.0.0' }
    if (-not [int]::TryParse($httpsEndpoint.Groups['port'].Value, [ref]$httpsPort) -or $httpsPort -lt 1 -or $httpsPort -gt 65535 -or $httpsPort -eq $listenPort) { throw 'Invalid server.httpsAddr port.' }
    foreach ($setting in @('httpsCertPath', 'httpsKeyPath')) {
        $pathMatch = [regex]::Match($configText, '(?m)^[ \t]+' + $setting + ':[ \t]*["'']?([^"''\r\n]+)')
        if (-not $pathMatch.Success) { throw "server.$setting is required for HTTPS." }
        $tlsPath = $pathMatch.Groups[1].Value.Trim()
        if (-not [IO.Path]::IsPathRooted($tlsPath)) { $tlsPath = Join-Path $projectRoot $tlsPath }
        if (-not (Test-Path -LiteralPath $tlsPath -PathType Leaf)) { throw "The configured $setting file is missing." }
    }
    $httpsSummary = '; HTTPS listen=' + $httpsMatch.Groups[1].Value.Trim()
}
function Test-HTTPSListener([int]$OwnerProcessId) {
    if ($httpsPort -eq 0) { return $true }
    $tlsListeners = @(Get-NetTCPConnection -LocalPort $httpsPort -State Listen -ErrorAction SilentlyContinue)
    return @($tlsListeners | Where-Object { $_.OwningProcess -eq $OwnerProcessId -and (Test-ConfiguredListenerAddress $_.LocalAddress $httpsHost $httpsPort) }).Count -gt 0
}
function Start-ConfiguredHTTPSProxy {
    $proxyBinary = Join-Path $projectRoot 'bin/caddy.exe'
    $proxyConfig = Join-Path $projectRoot 'manifest/https/Caddyfile'
    if ((Test-Path -LiteralPath $proxyBinary -PathType Leaf) -and (Test-Path -LiteralPath $proxyConfig -PathType Leaf)) {
        try {
            & (Join-Path $PSScriptRoot 'start-https.ps1') -Restart:$Restart
        } catch {
            throw "The HTTP host is running, but the HTTPS proxy failed: $($_.Exception.Message)"
        }
    }
}
$browserHost = $listenHost
if ($listenHost -eq '0.0.0.0') { $browserHost = '127.0.0.1' }
if ($listenHost -eq '::') { $browserHost = '::1' }
if ($browserHost.Contains(':')) { $browserHost = "[$browserHost]" }
$localURL = 'http://' + $browserHost + ':' + $listenPort + '/suxinweb/'
$listeners = @(Get-NetTCPConnection -LocalPort $listenPort -State Listen -ErrorAction SilentlyContinue)
# Check HTTPS conflicts before stopping the current HTTP service.
if ($httpsPort -gt 0) {
    foreach ($tlsListener in @(Get-NetTCPConnection -LocalPort $httpsPort -State Listen -ErrorAction SilentlyContinue)) {
        $tlsProcess = Get-Process -Id $tlsListener.OwningProcess -ErrorAction Stop
        if ($tlsProcess.Path -ne $binaryPath -or $listeners.OwningProcess -notcontains $tlsListener.OwningProcess) {
            throw "HTTPS port $httpsPort belongs to a different process; startup was canceled."
        }
    }
}
if ($listeners.Count -gt 0) {
    $processIds = @($listeners | Select-Object -ExpandProperty OwningProcess -Unique)
    foreach ($listenerProcessId in $processIds) {
        $runningProcess = Get-Process -Id $listenerProcessId -ErrorAction Stop
        if ($runningProcess.Path -ne $binaryPath) { throw "Port $listenPort belongs to a different application." }
    }
    if ($processIds.Count -ne 1) { throw "Multiple GoSuxin processes listen on port $listenPort; resolve the conflicting instances first." }
    $processId = [int]$processIds[0]
    if ($Restart) {
        if ($listenPort -notin @(9880, 19880)) { throw 'Automatic restart is restricted to this project on port 9880 or internal port 19880.' }
        $runningProcess = Get-Process -Id $processId -ErrorAction Stop
        if ($runningProcess.Path -ne $binaryPath) { throw 'The listener process changed; restart was canceled.' }
        Stop-Process -InputObject $runningProcess -ErrorAction Stop
        if (-not $runningProcess.WaitForExit(10000)) { throw 'The project process did not exit within 10 seconds.' }
        $remainingListeners = @(Get-NetTCPConnection -LocalPort $listenPort -State Listen -ErrorAction SilentlyContinue)
        if ($remainingListeners.Count -gt 0) { throw "Port $listenPort is still occupied; startup was canceled." }
    } else {
        $matchingListeners = @($listeners | Where-Object { Test-ConfiguredListenerAddress $_.LocalAddress })
        if ($matchingListeners.Count -eq 0) {
            $actualAddresses = ($listeners | Select-Object -ExpandProperty LocalAddress -Unique) -join ', '
            throw "GoSuxin PID $processId still listens on $actualAddresses. Run .\scripts\start-local.ps1 -Restart to apply server.address=$listenAddress."
        }
        if (-not (Test-HTTPSListener $processId)) { throw 'HTTPS is not running. Run .\scripts\start-local.ps1 -Restart to apply the TLS configuration.' }
        Start-ConfiguredHTTPSProxy
        Write-Output "GoSuxinV1.0 is running: HTTP listen=$listenAddress$httpsSummary; internal HTTP URL=$localURL (PID $processId)"
        exit 0
    }
}
$coreProcess = Start-Process -FilePath $binaryPath -WorkingDirectory $projectRoot -WindowStyle Hidden -RedirectStandardOutput (Join-Path $projectRoot 'runtime/core-running-stdout.log') -RedirectStandardError (Join-Path $projectRoot 'runtime/core-running-stderr.log') -PassThru
$coreProcess.Id | Set-Content -LiteralPath (Join-Path $projectRoot 'runtime/core-host.pid') -Encoding ASCII
for ($attempt = 0; $attempt -lt 20; $attempt++) {
    $listeners = @(Get-NetTCPConnection -LocalPort $listenPort -State Listen -ErrorAction SilentlyContinue)
    $matchingListeners = @($listeners | Where-Object { $_.OwningProcess -eq $coreProcess.Id -and (Test-ConfiguredListenerAddress $_.LocalAddress) })
    if ($matchingListeners.Count -gt 0 -and (Test-HTTPSListener $coreProcess.Id)) {
        Start-ConfiguredHTTPSProxy
        Write-Output "GoSuxinV1.0 started: HTTP listen=$listenAddress$httpsSummary; internal HTTP URL=$localURL (PID $($coreProcess.Id))"
        exit 0
    }
    if ($coreProcess.HasExited) { break }
    Start-Sleep -Milliseconds 250
}
throw 'Startup did not complete; inspect runtime/core-running-stdout.log and stderr.log.'
