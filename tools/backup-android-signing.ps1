param(
    [string]$BackupParent = 'E:\OpenIM38-PrivateBackups',
    [string]$ReportPath
)

$ErrorActionPreference = 'Stop'
$projectRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
if (-not $ReportPath) {
    $ReportPath = Join-Path $projectRoot 'tests\delivery\android-signing-backup-review.json'
}
$ReportPath = [IO.Path]::GetFullPath($ReportPath)
if (Test-Path -LiteralPath $ReportPath) { throw 'Report exists; preserve earlier evidence.' }
$privateParent = [IO.Path]::GetFullPath($BackupParent)
if ($privateParent.TrimEnd('\').Equals($projectRoot.TrimEnd('\'), [StringComparison]::OrdinalIgnoreCase) -or
    $privateParent.StartsWith($projectRoot.TrimEnd('\') + '\', [StringComparison]::OrdinalIgnoreCase)) {
    throw 'Signing backup must be outside the development source tree.'
}
$currentSid = [Security.Principal.WindowsIdentity]::GetCurrent().User
$systemSid = New-Object Security.Principal.SecurityIdentifier('S-1-5-18')
$allowedSids = @($currentSid.Value, $systemSid.Value)

function Set-PrivateAcl([string]$Path, [bool]$Directory) {
    if ($Directory) {
        $acl = New-Object Security.AccessControl.DirectorySecurity
        $inheritance = [Security.AccessControl.InheritanceFlags]'ContainerInherit, ObjectInherit'
    } else {
        $acl = New-Object Security.AccessControl.FileSecurity
        $inheritance = [Security.AccessControl.InheritanceFlags]::None
    }
    $acl.SetOwner($currentSid)
    $acl.SetAccessRuleProtection($true, $false)
    foreach ($sid in @($currentSid, $systemSid)) {
        $rule = New-Object Security.AccessControl.FileSystemAccessRule(
            $sid, [Security.AccessControl.FileSystemRights]::FullControl, $inheritance,
            [Security.AccessControl.PropagationFlags]::None,
            [Security.AccessControl.AccessControlType]::Allow)
        $acl.AddAccessRule($rule)
    }
    Set-Acl -LiteralPath $Path -AclObject $acl
}

function Assert-PrivateAcl([string]$Path) {
    $acl = Get-Acl -LiteralPath $Path
    if (-not $acl.AreAccessRulesProtected) { throw 'Backup ACL inheritance is not protected.' }
    $rules = @($acl.Access)
    if ($rules.Count -ne 2) { throw 'Unexpected backup ACL entries.' }
    $actualSids = @($rules | ForEach-Object {
        if ($_.AccessControlType -ne 'Allow' -or $_.IsInherited) { throw 'Unexpected backup ACL rule.' }
        $_.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value
    })
    if (@(Compare-Object ($allowedSids | Sort-Object) ($actualSids | Sort-Object)).Count) {
        throw 'Unexpected backup ACL principals.'
    }
}

# No secret is copied until the new destination directory has its final ACL.
if (-not (Test-Path -LiteralPath $privateParent)) {
    New-Item -ItemType Directory -Path $privateParent | Out-Null
    Set-PrivateAcl $privateParent $true
}
$backupName = 'android-signing-' + [DateTime]::UtcNow.ToString('yyyyMMddTHHmmssZ') + '-' + [Guid]::NewGuid().ToString('N').Substring(0, 8)
$backupDirectory = Join-Path $privateParent $backupName
New-Item -ItemType Directory -Path $backupDirectory | Out-Null
Set-PrivateAcl $backupDirectory $true
Assert-PrivateAcl $backupDirectory
$entries = @()
foreach ($name in @('key.properties', 'upgrade38-test.jks')) {
    $sourcePath = Join-Path $projectRoot ('clients\flutter\android\' + $name)
    $destinationPath = Join-Path $backupDirectory $name
    if (-not (Test-Path -LiteralPath $sourcePath -PathType Leaf)) { throw 'Signing input missing.' }
    if (Test-Path -LiteralPath $destinationPath) { throw 'Backup destination already exists.' }
    $before = (Get-FileHash -LiteralPath $sourcePath -Algorithm SHA256).Hash.ToLowerInvariant()
    Copy-Item -LiteralPath $sourcePath -Destination $destinationPath
    Set-PrivateAcl $destinationPath $false
    Assert-PrivateAcl $destinationPath
    $copied = (Get-FileHash -LiteralPath $destinationPath -Algorithm SHA256).Hash.ToLowerInvariant()
    $after = (Get-FileHash -LiteralPath $sourcePath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($before -ne $copied -or $before -ne $after) { throw 'Signing backup hash mismatch or source changed.' }
    $entries += [ordered]@{ name = $name; bytes = (Get-Item -LiteralPath $destinationPath).Length; sha256 = $copied; sourceUnchanged = $true; aclVerified = $true }
}
$manifestPath = Join-Path $backupDirectory 'manifest.json'
$manifest = [ordered]@{
    createdAt = [DateTime]::UtcNow.ToString('o'); source = $projectRoot
    scope = 'Private project signing inputs; never include in public installation packages.'
    files = $entries
}
[IO.File]::WriteAllText($manifestPath, ($manifest | ConvertTo-Json -Depth 5) + [Environment]::NewLine, (New-Object Text.UTF8Encoding($false)))
Set-PrivateAcl $manifestPath $false
Assert-PrivateAcl $manifestPath
$report = [ordered]@{
    createdAt = [DateTime]::UtcNow.ToString('o'); backupDirectory = $backupDirectory
    executionToolSHA256 = (Get-FileHash -LiteralPath $PSCommandPath -Algorithm SHA256).Hash.ToLowerInvariant()
    files = $entries; privateManifestAclVerified = $true; accepted = $true
    limits = @('A separate local directory on the same E: volume is not an off-device or offline backup.', 'No key, password or properties value is included in this public report.', 'The original signing inputs were not modified.')
}
[IO.File]::WriteAllText($ReportPath, ($report | ConvertTo-Json -Depth 5) + [Environment]::NewLine, (New-Object Text.UTF8Encoding($false)))
Write-Output ($report | ConvertTo-Json -Depth 5)
