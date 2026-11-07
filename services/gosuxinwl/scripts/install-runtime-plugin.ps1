param(
    [Parameter(Mandatory=$true)][string]$Archive,
    [Parameter(Mandatory=$true)][string]$Authorization,
    [Parameter(Mandatory=$true)][string]$ApiVerify,
    [string]$BaseUrl = 'http://127.0.0.1:8602'
)
$ErrorActionPreference = 'Stop'
# The caller supplies fresh signed headers from an authorized host session.
# Credentials are never written to a configuration file or console.
Add-Type -AssemblyName System.Net.Http
$client = [System.Net.Http.HttpClient]::new()
$client.Timeout = [TimeSpan]::FromMinutes(10)
$client.DefaultRequestHeaders.TryAddWithoutValidation('Authorization', $Authorization) | Out-Null
$client.DefaultRequestHeaders.TryAddWithoutValidation('apiverify', $ApiVerify) | Out-Null
$stream = [IO.File]::OpenRead((Resolve-Path -LiteralPath $Archive).Path)
$form = [System.Net.Http.MultipartFormDataContent]::new()
$form.Add([System.Net.Http.StreamContent]::new($stream), 'file', [IO.Path]::GetFileName($Archive))
try {
    $response = $client.PostAsync("$BaseUrl/admin/developer/packinstall/installLocalCode", $form).GetAwaiter().GetResult()
    $response.EnsureSuccessStatusCode() | Out-Null
    $body = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult() | ConvertFrom-Json
    if ($body.code -ne 0) { throw $body.message }
    if ($body.data -notmatch '^runtime-[0-9a-f]{48}$') { throw '请使用 suxin-runtime-v1 运行包' }
    $json = @{ name = $body.data } | ConvertTo-Json -Compress
    $content = [System.Net.Http.StringContent]::new($json, [Text.Encoding]::UTF8, 'application/json')
    $response = $client.PostAsync("$BaseUrl/admin/developer/packinstall/installCode", $content).GetAwaiter().GetResult()
    $body = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult() | ConvertFrom-Json
    if ($body.code -ne 0) { throw $body.message }
    Write-Output $body.message
} finally { $form.Dispose(); $stream.Dispose(); $client.Dispose() }
