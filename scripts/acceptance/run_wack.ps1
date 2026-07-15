[CmdletBinding()]
param(
  [Parameter(Mandatory = $true)]
  [ValidateScript({ Test-Path $_ -PathType Leaf })]
  [string]$PackagePath,

  [Parameter(Mandatory = $true)]
  [ValidatePattern('^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$')]
  [string]$RunId,

  [string]$WackPath = 'C:\Program Files (x86)\Windows Kits\10\App Certification Kit\appcert.exe',

  [string]$ExpectedPackageIdentity = 'ReluxWorksLLC.PulsarBarycenter'
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Assert-InteractiveAdministrator {
  $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
  $principal = [Security.Principal.WindowsPrincipal]::new($identity)
  if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'WACK must run from an elevated terminal.'
  }
  if ([Environment]::UserInteractive -ne $true -or
      $env:SESSIONNAME -eq 'Services' -or
      [Diagnostics.Process]::GetCurrentProcess().SessionId -eq 0) {
    throw 'WACK must run in the active interactive user session, never Session 0.'
  }
}

Assert-InteractiveAdministrator
if (-not (Test-Path $WackPath -PathType Leaf)) {
  throw "WACK executable missing: $WackPath"
}
Add-Type -AssemblyName System.IO.Compression.FileSystem

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$artifactRoot = Join-Path $repoRoot '.temp/acceptance'
$runRoot = Join-Path $artifactRoot $RunId
if (Test-Path $runRoot) {
  throw "Acceptance run already exists: $runRoot"
}
New-Item -ItemType Directory -Path $runRoot -Force:$false | Out-Null

$package = (Resolve-Path $PackagePath).Path
$archive = [IO.Compression.ZipFile]::OpenRead($package)
try {
  $manifestEntry = $archive.Entries | Where-Object FullName -CEQ 'AppxManifest.xml' | Select-Object -First 1
  if ($null -eq $manifestEntry) { throw 'MSIX has no root AppxManifest.xml' }
  $reader = [IO.StreamReader]::new($manifestEntry.Open())
  try { [xml]$appxManifest = $reader.ReadToEnd() } finally { $reader.Dispose() }
  $actualIdentity = $appxManifest.Package.Identity.Name
  if ($actualIdentity -cne $ExpectedPackageIdentity) {
    throw "Package identity '$actualIdentity' does not match '$ExpectedPackageIdentity'"
  }
} finally {
  $archive.Dispose()
}
$report = Join-Path $runRoot 'wack-report.xml'
$started = [DateTimeOffset]::UtcNow.ToString('o')

& $WackPath reset
if ($LASTEXITCODE -ne 0) { throw "appcert reset failed with exit code $LASTEXITCODE" }
& $WackPath test -appxpackagepath $package -reportoutputpath $report
$wackExitCode = $LASTEXITCODE
if ($wackExitCode -ne 0 -or -not (Test-Path $report -PathType Leaf)) {
  throw "WACK failed or produced no report (exit code $wackExitCode)"
}

$reportHash = (Get-FileHash -Algorithm SHA256 $report).Hash.ToLowerInvariant()
$packageHash = (Get-FileHash -Algorithm SHA256 $package).Hash.ToLowerInvariant()
$version = [Diagnostics.FileVersionInfo]::GetVersionInfo($WackPath).FileVersion
$manifest = [ordered]@{
  schemaVersion = 1
  scope = 'manual-wack-real-app'
  status = 'tool-completed-review-report'
  startedAt = $started
  finishedAt = [DateTimeOffset]::UtcNow.ToString('o')
  gitCommit = (& git -C $repoRoot rev-parse HEAD).Trim()
  package = [ordered]@{
    fileName = [IO.Path]::GetFileName($package)
    identity = $actualIdentity
    sha256 = $packageHash
  }
  wack = [ordered]@{
    version = $version
    exitCode = $wackExitCode
    report = 'wack-report.xml'
    reportSha256 = $reportHash
  }
  operatorReviewRequired = $true
}
$manifestPath = Join-Path $runRoot 'wack-manifest.json'
$manifest | ConvertTo-Json -Depth 6 | Set-Content -Encoding utf8NoBOM $manifestPath
Write-Output $manifestPath
