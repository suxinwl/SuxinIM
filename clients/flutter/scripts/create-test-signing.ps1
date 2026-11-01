param([string]$Jdk = 'E:\AndroidDev\jdk\jdk-17.0.20.1+1')
$ErrorActionPreference = 'Stop'
$androidRoot = Join-Path (Split-Path $PSScriptRoot -Parent) 'android'
$keystore = Join-Path $androidRoot 'upgrade38-test.jks'
$properties = Join-Path $androidRoot 'key.properties'
if ((Test-Path -LiteralPath $keystore) -or (Test-Path -LiteralPath $properties)) {
  if ((Test-Path -LiteralPath $keystore) -and (Test-Path -LiteralPath $properties)) {
    Write-Output 'Existing project signing key retained.'
    return
  }
  throw 'Incomplete signing configuration: restore the matching private backup. No file was replaced.'
}
$keytool = Join-Path $Jdk 'bin/keytool.exe'
if (-not (Test-Path -LiteralPath $keytool)) { throw 'JDK 17 keytool not found.' }
$randomBytes = New-Object byte[] 36
$random = [Security.Cryptography.RandomNumberGenerator]::Create()
try { $random.GetBytes($randomBytes) } finally { $random.Dispose() }
$secret = [Convert]::ToBase64String($randomBytes)
$previousStore = $env:OPENIM38_STORE_PASSWORD
try {
  $env:OPENIM38_STORE_PASSWORD = $secret
  & $keytool -genkeypair -alias openim38-test -keyalg RSA -keysize 3072 -validity 3650 -storetype JKS -keystore $keystore -storepass:env OPENIM38_STORE_PASSWORD -keypass:env OPENIM38_STORE_PASSWORD -dname 'CN=OpenIM38 Isolated Development,O=Local Development,C=CN' -noprompt
  if ($LASTEXITCODE) { throw 'Key generation failed.' }
  [IO.File]::WriteAllText($properties, "storePassword=$secret`nkeyPassword=$secret`nkeyAlias=openim38-test`nstoreFile=../upgrade38-test.jks`n")
  $identity = [Security.Principal.WindowsIdentity]::GetCurrent().Name
  foreach ($file in @($keystore,$properties)) {
    $acl = New-Object Security.AccessControl.FileSecurity
    $acl.SetAccessRuleProtection($true,$false)
    $acl.AddAccessRule((New-Object Security.AccessControl.FileSystemAccessRule($identity,'FullControl','Allow')))
    $acl.AddAccessRule((New-Object Security.AccessControl.FileSystemAccessRule('SYSTEM','FullControl','Allow')))
    Set-Acl -LiteralPath $file -AclObject $acl
  }
  Write-Output 'Project test signing generated. Back up both private files together; keep them out of source packages.'
} finally {
  $env:OPENIM38_STORE_PASSWORD = $previousStore
  $secret = $null
}
