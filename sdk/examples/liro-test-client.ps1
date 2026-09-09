$ErrorActionPreference = "Stop"
$inv  = [Globalization.CultureInfo]::InvariantCulture
$utf8 = New-Object Text.UTF8Encoding($false)

# ---------------------------------------------------------------- find agent
$disco = Join-Path $env:LOCALAPPDATA "Liro\bridge.json"
if (-not (Test-Path $disco)) { throw "Agent not running: $disco not found" }
$port = (Get-Content $disco -Raw | ConvertFrom-Json).port
$base = "http://127.0.0.1:$port"
Write-Host "Agent on port $port"

# ---------------------------------------------------------------- helpers
function Sha256Hex([byte[]]$bytes) {
  if ($null -eq $bytes) { $bytes = [byte[]]::new(0) }
  $sha  = [Security.Cryptography.SHA256]::Create()
  $hash = $sha.ComputeHash($bytes, 0, $bytes.Length)
  ($hash | ForEach-Object { $_.ToString("x2") }) -join ""
}

function Raw($method, $path, $bodyBytes, $headers) {
  $req = [Net.HttpWebRequest]::Create("$base$path")
  $req.Method = $method.ToUpper()
  foreach ($k in $headers.Keys) { $req.Headers.Add($k, $headers[$k]) }

  if ($bodyBytes -and $bodyBytes.Length -gt 0) {
    $req.ContentType   = "application/json"
    $req.ContentLength = $bodyBytes.Length
    $s = $req.GetRequestStream()
    $s.Write($bodyBytes, 0, $bodyBytes.Length)
    $s.Close()
  }

  try {
    $resp = $req.GetResponse()
  } catch [Net.WebException] {
    $resp = $_.Exception.Response
    if ($null -eq $resp) { throw }
  }

  $sr   = New-Object IO.StreamReader($resp.GetResponseStream())
  $text = $sr.ReadToEnd()
  $sr.Close()
  $code = [int]$resp.StatusCode
  $resp.Close()

  [pscustomobject]@{ StatusCode = $code; Content = $text }
}

function Send($method, $path, $bodyObj, $appId, $secret) {
  $bodyBytes = if ($null -eq $bodyObj) { [byte[]]::new(0) }
               else { $utf8.GetBytes(($bodyObj | ConvertTo-Json -Depth 10 -Compress)) }

  $ts    = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds().ToString($inv)
  $nonce = [Guid]::NewGuid().ToString("N")
  $canon = ($method.ToUpper(), $path, $ts, $nonce, (Sha256Hex $bodyBytes)) -join "`n"

  $hmac = New-Object Security.Cryptography.HMACSHA256(,[Convert]::FromBase64String($secret))
  $sig  = ($hmac.ComputeHash($utf8.GetBytes($canon)) | ForEach-Object { $_.ToString("x2") }) -join ""

  Raw $method $path $bodyBytes @{
    "X-Liro-App-Id"    = $appId
    "X-Liro-Timestamp" = $ts
    "X-Liro-Nonce"     = $nonce
    "X-Liro-Signature" = $sig
  }
}

# ---------------------------------------------------------------- health
Write-Host "--- health ---"
$h = (Raw GET "/v2/health" $null @{}).Content | ConvertFrom-Json
$h | Format-List

# ---------------------------------------------------------------- pairing
$appId  = $env:LIRO_APP_ID
$secret = $env:LIRO_SECRET
$origin = "https://test.local"

if (-not $appId) {
  $b = $utf8.GetBytes((@{ applicationName = "Test klijent"; origin = $origin } | ConvertTo-Json -Compress))
  $r = Raw POST "/v2/pair/request" $b @{}
  if ($r.StatusCode -ne 200) { throw "pair/request: $($r.Content)" }
  $req = $r.Content | ConvertFrom-Json

  Write-Host ""
  Write-Host "Read the six-digit code from the agent window."
  $code = Read-Host "Code"

  $b = $utf8.GetBytes((@{ requestId = $req.requestId; code = $code; origin = $origin } | ConvertTo-Json -Compress))
  $r = Raw POST "/v2/pair/confirm" $b @{}
  if ($r.StatusCode -ne 200) { throw "pair/confirm: $($r.Content)" }
  $ok = $r.Content | ConvertFrom-Json

  $appId = $ok.appId; $secret = $ok.deviceSecret
  Write-Host ""
  Write-Host "Save these for next time:"
  Write-Host ('  $env:LIRO_APP_ID = "' + $appId + '"')
  Write-Host ('  $env:LIRO_SECRET = "' + $secret + '"')
  Write-Host ""
}

# ---------------------------------------------------------------- submit
$pdf = Resolve-Path $args[0]
$body = @{
  documents = @(@{
    name    = [IO.Path]::GetFileName($pdf)
    content = [Convert]::ToBase64String([IO.File]::ReadAllBytes($pdf))
  })
  level = "b-b"
}

$r = Send POST "/v2/sign/pdf" $body $appId $secret
if ($r.StatusCode -ne 202) { throw "sign/pdf: HTTP $($r.StatusCode) $($r.Content)" }
$job = $r.Content | ConvertFrom-Json

Write-Host ("Job " + $job.jobId + " - approve in the agent window and enter the PIN.")
Write-Host ("Batch fingerprint: " + $job.batchFingerprint)
Write-Host ""

# ---------------------------------------------------------------- wait
while ($true) {
  Start-Sleep -Seconds 1
  $resp = Send GET ("/v2/jobs/" + $job.jobId + "/result") $null $appId $secret

  if ($resp.StatusCode -eq 202) {
    $s = $resp.Content | ConvertFrom-Json
    Write-Host ("  " + $s.state + "  " + $s.completed + "/" + $s.total)
    continue
  }

  if ($resp.StatusCode -ne 200) {
    Write-Host ("Failed: HTTP " + $resp.StatusCode + "  " + $resp.Content)
    break
  }

  $res = $resp.Content | ConvertFrom-Json
  $out = Join-Path (Split-Path $pdf) "protokol-potpisan.pdf"
  [IO.File]::WriteAllBytes($out, [Convert]::FromBase64String($res.documents[0].content))
  Write-Host ""
  Write-Host ("Saved:  " + $out)
  Write-Host ("Level:  " + $res.documents[0].achievedLevel)
  break
}