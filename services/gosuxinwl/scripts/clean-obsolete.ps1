param([switch]$Apply)
$ErrorActionPreference = 'Stop'
$workspaceRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path.TrimEnd('\')
$rootPrefix = $workspaceRoot + [IO.Path]::DirectorySeparatorChar

# Only these confirmed, one-off files may be removed. Never enumerate by extension.
$relativeTargets = @(
    'runtime/core-backend-check.json',
    'runtime/core-database-ready.json',
    'runtime/core-frontend-check.json',
    'runtime/core-frontend-check.log',
    'runtime/core-install-result.json',
    'runtime/core-startup-home.png',
    'runtime/core-startup-native-ai.png',
    'runtime/core-startup-plugin-market.png',
    'runtime/core-startup-result.json',
    'runtime/core-startup-stderr.log',
    'runtime/core-startup-stdout.log',
    'runtime/core-startup-ui-check.json',
    'runtime/startup-db-check.json',
    'runtime/verify_core_startup_ui.cjs'
)

function Get-CleanupFile([string]$RelativePath) {
    $path = [IO.Path]::GetFullPath((Join-Path $workspaceRoot $RelativePath))
    if (-not $path.StartsWith($rootPrefix, [StringComparison]::OrdinalIgnoreCase)) {
        throw "Target escapes workspace: $RelativePath"
    }
    if (-not (Test-Path -LiteralPath $path)) { return }
    $item = Get-Item -LiteralPath $path -Force
    if ($item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
        Write-Warning "Skipped directory or link: $RelativePath"
        return
    }
    # Check parent directories as well: an in-project junction can point elsewhere.
    $directory = $item.Directory
    while ($null -ne $directory -and -not $directory.FullName.Equals($workspaceRoot, [StringComparison]::OrdinalIgnoreCase)) {
        if ($directory.Attributes -band [IO.FileAttributes]::ReparsePoint) {
            Write-Warning "Skipped file beneath a directory link: $RelativePath"
            return
        }
        $directory = $directory.Parent
    }
    if ($null -eq $directory) { throw "Target is outside workspace: $RelativePath" }
    $stream = $null
    try {
        $stream = [IO.File]::Open($path, [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::None)
    } catch [IO.IOException] {
        Write-Warning "Skipped file in use: $RelativePath"
        return
    } catch [UnauthorizedAccessException] {
        Write-Warning "Skipped inaccessible file: $RelativePath"
        return
    } finally {
        if ($null -ne $stream) { $stream.Dispose() }
    }
    return $item
}

$targets = @()
$bytes = [long]0
foreach ($relative in $relativeTargets) {
    $item = Get-CleanupFile $relative
    if ($null -eq $item) { continue }
    $targets += $relative
    $bytes += $item.Length
    Write-Output $relative
}
Write-Output ("Targets: {0}; size: {1:N2} MiB" -f $targets.Count, ($bytes / 1MB))
if (-not $Apply) {
    Write-Output 'Preview only. Run with -Apply to remove the listed files.'
    exit
}

$removed = 0
foreach ($relative in $targets) {
    # Recheck the boundary, links and file availability immediately before removal.
    $item = Get-CleanupFile $relative
    if ($null -eq $item) { continue }
    try {
        Remove-Item -LiteralPath $item.FullName -Force -ErrorAction Stop
        $removed++
    } catch {
        Write-Warning "Skipped file that could not be removed: $relative"
    }
}
Write-Output ("Cleanup completed: {0} files removed. Current logs, PID, configuration and business data are retained." -f $removed)
