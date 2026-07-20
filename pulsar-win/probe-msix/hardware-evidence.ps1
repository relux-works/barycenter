[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidateSet("Initialize", "Snapshot", "Attach", "Verdict", "Seal")]
    [string]$Mode,
    [string]$OutputDirectory = "",
    [string]$RunID = "",
    [ValidateSet("windows10", "windows11")][string]$OSFamily = "windows11",
    [string]$Package = "",
    [switch]$PhysicalMachineAttested,
    [switch]$ConsoleOperatorAttested,
    [string]$OutputEndpointName = "",
    [string]$DefaultInputName = "",
    [string]$SelectedInputName = "",
    [switch]$SingleInputApprovedException,
    [string]$SingleInputDecisionReference = "",
    [ValidateSet("EnterpriseLTSC2021", "ApprovedException")]
    [string]$Windows10Posture = "EnterpriseLTSC2021",
    [string]$SupportDecisionReference = "",
    [ValidateSet("H00", "H01", "H02", "H03", "H04", "H05", "H06", "H07", "H08", "H09", "H10", "H11", "H12", "H13", "H14", "H15", "H16", "H17")]
    [string]$Scenario = "H00",
    [string]$Attachment = "",
    [ValidateSet("PASS", "FAIL", "BLOCKED")][string]$Verdict = "PASS",
    [string]$Observation = "",
    [string]$NextAction = ""
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest
. (Join-Path $PSScriptRoot "hardware-evidence-contract.ps1")

$ScenarioTitles = [ordered]@{
    H00 = "clean-signed-install"
    H01 = "cold-permission-deny"
    H02 = "cold-permission-allow"
    H03 = "default-capture"
    H04 = "selected-capture"
    H05 = "hotkey-and-conflict"
    H06 = "brokered-picker"
    H07 = "hidden-window-recording"
    H08 = "repeated-cycles"
    H09 = "quit-during-capture"
    H10 = "suspend-resume"
    H11 = "session-lock-unlock"
    H12 = "device-removal"
    H13 = "permission-revoke-restore"
    H14 = "abrupt-kill-recovery"
    H15 = "signout-shutdown-boundary"
    H16 = "wack"
    H17 = "final-cleanup"
}

function Get-EvidenceRoot {
    if ([string]::IsNullOrWhiteSpace($OutputDirectory)) {
        throw "-OutputDirectory is required for every hardware evidence mode"
    }
    if (-not (Test-Path -LiteralPath $OutputDirectory -PathType Container)) {
        throw "evidence output directory does not exist; use Initialize first"
    }
    (Resolve-Path -LiteralPath $OutputDirectory).Path
}

function Get-EvidenceRun {
    $Root = Get-EvidenceRoot
    $RunPath = Join-Path $Root "run.json"
    $MatrixPath = Join-Path $Root "matrix.json"
    $MachinePath = Join-Path $Root "machine.json"
    $PackagePath = Join-Path $Root "package.json"
    if (-not (Test-Path -LiteralPath $RunPath -PathType Leaf) -or
        -not (Test-Path -LiteralPath $MatrixPath -PathType Leaf) -or
        -not (Test-Path -LiteralPath $MachinePath -PathType Leaf) -or
        -not (Test-Path -LiteralPath $PackagePath -PathType Leaf)) {
        throw "evidence directory lacks the frozen run, machine, package, or matrix contract"
    }
    $Run = Get-Content -LiteralPath $RunPath -Raw | ConvertFrom-Json
    $Matrix = Get-Content -LiteralPath $MatrixPath -Raw | ConvertFrom-Json
    $Machine = Get-Content -LiteralPath $MachinePath -Raw | ConvertFrom-Json
    $PackageEvidence = Get-Content -LiteralPath $PackagePath -Raw | ConvertFrom-Json
    if ([int]$Run.schemaVersion -ne 1 -or
        [int]$Matrix.schemaVersion -ne 1 -or
        [int]$Machine.schemaVersion -ne 1 -or
        [int]$PackageEvidence.schemaVersion -ne 1) {
        throw "unsupported hardware evidence schema version"
    }
    Assert-ProbeEvidenceRunID -RunID ([string]$Run.runId) | Out-Null
    if ([string]$Run.osFamily -cnotin @("windows10", "windows11") -or
        [string]$Run.packageSha256 -cnotmatch '^[0-9a-f]{64}$' -or
        @($Matrix.scenarios).Count -ne $script:ProbeEvidenceScenarios.Count) {
        throw "run identity, OS family, package hash, or scenario count is invalid"
    }
    $ActualScenarioOrder = @($Matrix.scenarios | ForEach-Object { [string]$_.id }) -join ','
    if ($ActualScenarioOrder -cne ($script:ProbeEvidenceScenarios -join ',')) {
        throw "matrix scenario IDs or order differ from the frozen H00-H17 sequence"
    }
    if ([string]$Run.runId -cne [string]$Matrix.runId -or
        [string]$Run.osFamily -cne [string]$Matrix.osFamily -or
        [string]$Run.osFamily -cne [string]$Machine.osFamily -or
        [string]$Run.packageSha256 -cne [string]$Matrix.packageSha256 -or
        [string]$Run.packageSha256 -cne [string]$PackageEvidence.sha256) {
        throw "run, machine, package, and matrix provenance differ"
    }
    if ([string]$Run.packageIdentity -cne $script:ProbePackageIdentity -or
        [string]$Run.packageIdentity -cne [string]$PackageEvidence.packageIdentity -or
        [string]$Run.packageFamilyName -cne [string]$PackageEvidence.packageFamilyName -or
        [string]$Run.applicationUserModelId -cne [string]$PackageEvidence.applicationUserModelId -or
        [bool]$PackageEvidence.privateSigningMaterialIncluded) {
        throw "run and package evidence differ from the frozen signed package identity"
    }
    [pscustomobject]@{
        Root = $Root
        RunPath = $RunPath
        MatrixPath = $MatrixPath
        Run = $Run
        Matrix = $Matrix
        Machine = $Machine
        PackageEvidence = $PackageEvidence
    }
}

function Get-MatrixRow {
    param(
        [Parameter(Mandatory = $true)]$Matrix,
        [Parameter(Mandatory = $true)][string]$ScenarioID
    )
    Assert-ProbeEvidenceScenario -Scenario $ScenarioID | Out-Null
    $Rows = @($Matrix.scenarios | Where-Object { [string]$_.id -ceq $ScenarioID })
    if ($Rows.Count -ne 1) {
        throw "matrix must contain exactly one row for $ScenarioID"
    }
    $Rows[0]
}

function Assert-CurrentEvidenceScenario {
    param(
        [Parameter(Mandatory = $true)]$Matrix,
        [Parameter(Mandatory = $true)][string]$ScenarioID
    )
    $Pending = @($Matrix.scenarios | Where-Object { [string]$_.verdict -ceq "NOT_RUN" })
    if ($Pending.Count -eq 0) {
        throw "all frozen hardware scenarios already have terminal operator verdicts"
    }
    $Expected = [string]$Pending[0].id
    if ($ScenarioID -cne $Expected) {
        throw "strict current evidence scenario is $Expected; refusing out-of-order $ScenarioID"
    }
}

function Add-MatrixEvidence {
    param(
        [Parameter(Mandatory = $true)]$State,
        [Parameter(Mandatory = $true)][string]$ScenarioID,
        [Parameter(Mandatory = $true)]$Evidence
    )
    $Row = Get-MatrixRow -Matrix $State.Matrix -ScenarioID $ScenarioID
    if ([string]$Row.verdict -cne "NOT_RUN") {
        throw "matrix row $ScenarioID is already terminal; evidence cannot be appended"
    }
    $Existing = @($Row.evidence | Where-Object { [string]$_.relativeFile -ceq [string]$Evidence.relativeFile })
    if ($Existing.Count -ne 0) {
        throw "matrix row $ScenarioID already references '$($Evidence.relativeFile)'"
    }
    $Row.evidence = @($Row.evidence) + @($Evidence)
    $Row.collectionState = "collected-unreviewed"
    Write-ProbeEvidenceJSON -Value $State.Matrix -Path $State.MatrixPath -Replace
}

function Initialize-EvidenceRun {
    if ([string]::IsNullOrWhiteSpace($OutputDirectory) -or
        [string]::IsNullOrWhiteSpace($Package) -or
        [string]::IsNullOrWhiteSpace($RunID)) {
        throw "Initialize requires -OutputDirectory, -Package, and -RunID"
    }
    Assert-ProbeEvidenceRunID -RunID $RunID | Out-Null
    if (Test-Path -LiteralPath $OutputDirectory) {
        throw "Initialize requires a new evidence directory; refusing to splice an existing run"
    }
    if (@(Get-AppxPackage -Name $script:ProbePackageIdentity -ErrorAction SilentlyContinue).Count -ne 0) {
        throw "an existing package in the real Pulsar product family must be removed before H00"
    }

    $PackageEvidence = Get-ProbePackageEvidence -Package $Package
    if ([string]$PackageEvidence.signerRoute -ceq "self-signed-controlled-hardware") {
        $SignerTrustPath = "Cert:\LocalMachine\TrustedPeople\$(([string]$PackageEvidence.signerThumbprint).ToUpperInvariant())"
        if (Test-Path -LiteralPath $SignerTrustPath) {
            throw "the local test signer is already trusted; H00 requires a clean trust state"
        }
    }
    $HostEvidence = Get-ProbeHardwareHostEvidence `
        -OSFamily $OSFamily `
        -PhysicalMachineAttested $PhysicalMachineAttested.IsPresent `
        -ConsoleOperatorAttested $ConsoleOperatorAttested.IsPresent `
        -OutputEndpointName $OutputEndpointName `
        -DefaultInputName $DefaultInputName `
        -SelectedInputName $SelectedInputName `
        -SingleInputApprovedException $SingleInputApprovedException.IsPresent `
        -SingleInputDecisionReference $SingleInputDecisionReference `
        -Windows10Posture $Windows10Posture `
        -SupportDecisionReference $SupportDecisionReference

    New-Item -ItemType Directory -Path $OutputDirectory | Out-Null
    $Root = (Resolve-Path -LiteralPath $OutputDirectory).Path
    New-Item -ItemType Directory -Path (Join-Path $Root "snapshots"), (Join-Path $Root "attachments") | Out-Null

    $Run = [ordered]@{
        schemaVersion = 1
        verificationBoundary = "physical-run-container-only; all scenario verdicts remain unreviewed"
        runId = $RunID
        osFamily = $OSFamily
        createdAtUTC = [DateTime]::UtcNow.ToString("o")
        packageSha256 = [string]$PackageEvidence.sha256
        packageIdentity = [string]$PackageEvidence.packageIdentity
        packageFamilyName = [string]$PackageEvidence.packageFamilyName
        applicationUserModelId = [string]$PackageEvidence.applicationUserModelId
    }
    $Rows = foreach ($ScenarioID in $script:ProbeEvidenceScenarios) {
        [ordered]@{
            id = $ScenarioID
            title = $ScenarioTitles[$ScenarioID]
            collectionState = "not-collected"
            verdict = "NOT_RUN"
            observation = $null
            nextAction = $null
            reviewState = "unreviewed"
            recordedAtUTC = $null
            evidence = @()
        }
    }
    $Matrix = [ordered]@{
        schemaVersion = 1
        verificationBoundary = "operator-matrix; verdicts are not accepted task evidence until root review"
        runId = $RunID
        osFamily = $OSFamily
        packageSha256 = [string]$PackageEvidence.sha256
        scenarios = @($Rows)
    }
    Write-ProbeEvidenceJSON -Value $Run -Path (Join-Path $Root "run.json")
    Write-ProbeEvidenceJSON -Value $HostEvidence -Path (Join-Path $Root "machine.json")
    Write-ProbeEvidenceJSON -Value $PackageEvidence -Path (Join-Path $Root "package.json")
    Write-ProbeEvidenceJSON -Value $Matrix -Path (Join-Path $Root "matrix.json")
    [pscustomobject]@{
        RunID = $RunID
        OutputDirectory = $Root
        PackageSHA256 = [string]$PackageEvidence.sha256
        Boundary = "initialized-only; no scenario passed"
    }
}

function Save-EvidenceSnapshot {
    $State = Get-EvidenceRun
    Assert-ProbeEvidenceScenario -Scenario $Scenario | Out-Null
    Assert-CurrentEvidenceScenario -Matrix $State.Matrix -ScenarioID $Scenario
    if ($Scenario -ceq "H17") {
        throw "H17 occurs after package removal; attach uninstall-probe cleanup JSON instead"
    }
    if (@(Get-Process -Name "pulsar-win-probe-amd64" -ErrorAction SilentlyContinue).Count -ne 0) {
        throw "exit the probe before taking a stable scenario snapshot"
    }
    $Packages = @(Get-AppxPackage -Name $script:ProbePackageIdentity -ErrorAction SilentlyContinue)
    if ($Packages.Count -ne 1) {
        throw "snapshot requires exactly one installed frozen probe package"
    }
    $Installed = $Packages[0]
    if ([string]$Installed.PackageFamilyName -cne [string]$State.Run.packageFamilyName) {
        throw "installed package family differs from the frozen evidence run"
    }

    $SnapshotRoot = Join-Path $State.Root "snapshots\$Scenario"
    if (Test-Path -LiteralPath $SnapshotRoot) {
        throw "scenario $Scenario already has a snapshot; never overwrite or splice evidence"
    }
    New-Item -ItemType Directory -Path $SnapshotRoot | Out-Null
    $RuntimeRoot = Join-Path $env:LOCALAPPDATA "Packages\$($Installed.PackageFamilyName)\LocalState\PulsarProbe"
    $ScenarioLog = Join-Path $RuntimeRoot "scenarios.jsonl"
    if (-not (Test-Path -LiteralPath $ScenarioLog -PathType Leaf)) {
        throw "installed probe scenario log is missing"
    }
    Copy-Item -LiteralPath $ScenarioLog -Destination (Join-Path $SnapshotRoot "scenarios.jsonl")
    $TargetEvidence = Join-Path $SnapshotRoot "evidence"
    New-Item -ItemType Directory -Path $TargetEvidence | Out-Null
    $SourceEvidence = Join-Path $RuntimeRoot "evidence"
    if (Test-Path -LiteralPath $SourceEvidence -PathType Container) {
        Get-ChildItem -LiteralPath $SourceEvidence -Force |
            Copy-Item -Destination $TargetEvidence -Recurse -Force
    }
    $LogContract = Assert-ProbeEvidenceJSONL -Path (Join-Path $SnapshotRoot "scenarios.jsonl")
    $Files = Get-ProbeEvidenceFileManifest -Root $SnapshotRoot
    $Manifest = [ordered]@{
        schemaVersion = 1
        verificationBoundary = "stable-copy-after-process-exit; scenario verdict remains unreviewed"
        scenario = $Scenario
        collectedAtUTC = [DateTime]::UtcNow.ToString("o")
        packageSha256 = [string]$State.Run.packageSha256
        eventCount = [int]$LogContract.EventCount
        firstEventUTC = [string]$LogContract.FirstTimestampUTC
        lastEventUTC = [string]$LogContract.LastTimestampUTC
        files = @($Files)
    }
    $ManifestPath = Join-Path $SnapshotRoot "manifest.json"
    Write-ProbeEvidenceJSON -Value $Manifest -Path $ManifestPath
    Add-MatrixEvidence -State $State -ScenarioID $Scenario -Evidence ([pscustomobject]@{
        kind = "runtime-snapshot"
        relativeFile = "snapshots/$Scenario/manifest.json"
        sha256 = (Get-FileHash -LiteralPath $ManifestPath -Algorithm SHA256).Hash.ToLowerInvariant()
    })
    $Manifest
}

function Add-EvidenceAttachment {
    $State = Get-EvidenceRun
    Assert-ProbeEvidenceScenario -Scenario $Scenario | Out-Null
    Assert-CurrentEvidenceScenario -Matrix $State.Matrix -ScenarioID $Scenario
    if ([string]::IsNullOrWhiteSpace($Attachment) -or
        -not (Test-Path -LiteralPath $Attachment -PathType Leaf)) {
        throw "Attach requires one existing evidence file"
    }
    $Source = (Resolve-Path -LiteralPath $Attachment).Path
    $SourceItem = Get-Item -LiteralPath $Source
    if (($SourceItem.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "evidence attachments may not be reparse points"
    }
    $Name = [IO.Path]::GetFileName($Source)
    Assert-ProbeEvidenceFileName -Name $Name | Out-Null
    $TargetDirectory = Join-Path $State.Root "attachments\$Scenario"
    New-Item -ItemType Directory -Force -Path $TargetDirectory | Out-Null
    $Target = Join-Path $TargetDirectory $Name
    if (Test-Path -LiteralPath $Target) {
        throw "attachment '$Name' already exists for $Scenario"
    }
    $Extension = [IO.Path]::GetExtension($Source).ToLowerInvariant()
    if ($Extension -ceq ".jsonl") {
        Assert-ProbeEvidenceJSONL -Path $Source | Out-Null
    } elseif ($Extension -ceq ".json") {
        $Parsed = Get-Content -LiteralPath $Source -Raw | ConvertFrom-Json
        Assert-ProbeEvidenceValueSafe -Value $Parsed
    } elseif ($Extension -cin @(".csv", ".log", ".md", ".txt", ".xml")) {
        Assert-ProbeEvidenceValueSafe -Value (Get-Content -LiteralPath $Source -Raw) -PropertyName "attachmentText"
    }
    Copy-Item -LiteralPath $Source -Destination $Target
    $Hash = (Get-FileHash -LiteralPath $Target -Algorithm SHA256).Hash.ToLowerInvariant()
    Add-MatrixEvidence -State $State -ScenarioID $Scenario -Evidence ([pscustomobject]@{
        kind = "attachment"
        relativeFile = "attachments/$Scenario/$Name"
        sha256 = $Hash
    })
    [pscustomobject]@{ Scenario = $Scenario; File = $Name; SHA256 = $Hash }
}

function Record-EvidenceVerdict {
    $State = Get-EvidenceRun
    $Row = Get-MatrixRow -Matrix $State.Matrix -ScenarioID $Scenario
    if ([string]$Row.verdict -cne "NOT_RUN") {
        throw "scenario $Scenario already has a terminal operator verdict"
    }
    Assert-CurrentEvidenceScenario -Matrix $State.Matrix -ScenarioID $Scenario
    Assert-ProbeScenarioVerdictForInputPosture `
        -InputPosture ([string]$State.Machine.inputPosture) `
        -Scenario $Scenario `
        -Verdict $Verdict
    if (@($Row.evidence).Count -eq 0) {
        throw "scenario $Scenario cannot receive a verdict without attached evidence"
    }
    if ([string]::IsNullOrWhiteSpace($Observation) -or $Observation.Length -gt 2000) {
        throw "Verdict requires a concise non-empty observation up to 2000 characters"
    }
    Assert-ProbeEvidenceValueSafe -Value $Observation -PropertyName "observation"
    if ($Verdict -ceq "PASS") {
        if (-not [string]::IsNullOrWhiteSpace($NextAction)) {
            throw "PASS must not carry a failure next action"
        }
    } else {
        if ([string]::IsNullOrWhiteSpace($NextAction) -or $NextAction.Length -gt 1000) {
            throw "$Verdict requires a concrete next action up to 1000 characters"
        }
        Assert-ProbeEvidenceValueSafe -Value $NextAction -PropertyName "nextAction"
    }
    $Row.verdict = $Verdict
    $Row.observation = $Observation
    $Row.nextAction = if ([string]::IsNullOrWhiteSpace($NextAction)) { $null } else { $NextAction }
    $Row.reviewState = "unreviewed"
    $Row.recordedAtUTC = [DateTime]::UtcNow.ToString("o")
    Write-ProbeEvidenceJSON -Value $State.Matrix -Path $State.MatrixPath -Replace
    [pscustomobject]@{
        Scenario = $Scenario
        Verdict = $Verdict
        Boundary = "operator verdict only; root acceptance not implied"
    }
}

function Assert-EvidenceReferencesIntact {
    param([Parameter(Mandatory = $true)]$State)

    $SeenReferences = [Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
    foreach ($Row in @($State.Matrix.scenarios)) {
        $ScenarioID = [string]$Row.id
        foreach ($Reference in @($Row.evidence)) {
            foreach ($Required in @("kind", "relativeFile", "sha256")) {
                if ($null -eq $Reference.PSObject.Properties[$Required] -or
                    [string]::IsNullOrWhiteSpace([string]$Reference.$Required)) {
                    throw "scenario $ScenarioID has an incomplete evidence reference"
                }
            }
            $Kind = [string]$Reference.kind
            $Relative = Assert-ProbeEvidenceRelativeFile -RelativeFile ([string]$Reference.relativeFile)
            if ($Kind -ceq "attachment") {
                if (-not $Relative.StartsWith("attachments/$ScenarioID/", [StringComparison]::Ordinal)) {
                    throw "scenario $ScenarioID attachment points outside its frozen directory"
                }
            } elseif ($Kind -ceq "runtime-snapshot") {
                if ($Relative -cne "snapshots/$ScenarioID/manifest.json") {
                    throw "scenario $ScenarioID runtime snapshot points outside its frozen manifest"
                }
            } else {
                throw "scenario $ScenarioID has unknown evidence reference kind '$Kind'"
            }
            if (-not $SeenReferences.Add($Relative)) {
                throw "evidence reference '$Relative' is duplicated"
            }
            if ([string]$Reference.sha256 -cnotmatch '^[0-9a-f]{64}$') {
                throw "scenario $ScenarioID evidence reference has an invalid SHA-256"
            }
            $ReferencePath = Join-Path $State.Root $Relative.Replace('/', [IO.Path]::DirectorySeparatorChar)
            if (-not (Test-Path -LiteralPath $ReferencePath -PathType Leaf)) {
                throw "scenario $ScenarioID referenced evidence file is missing"
            }
            $ActualHash = (Get-FileHash -LiteralPath $ReferencePath -Algorithm SHA256).Hash.ToLowerInvariant()
            if ($ActualHash -cne [string]$Reference.sha256) {
                throw "scenario $ScenarioID referenced evidence hash changed after collection"
            }

            if ($Kind -ceq "runtime-snapshot") {
                $Snapshot = Get-Content -LiteralPath $ReferencePath -Raw | ConvertFrom-Json
                if ([int]$Snapshot.schemaVersion -ne 1 -or
                    [string]$Snapshot.scenario -cne $ScenarioID -or
                    [string]$Snapshot.packageSha256 -cne [string]$State.Run.packageSha256) {
                    throw "scenario $ScenarioID runtime snapshot provenance differs from the frozen run"
                }
                $SnapshotRoot = Split-Path -Parent $ReferencePath
                $DeclaredFiles = @($Snapshot.files)
                $ActualFiles = @(Get-ProbeEvidenceFileManifest `
                    -Root $SnapshotRoot `
                    -ExcludeRelativePath @("manifest.json"))
                if ($DeclaredFiles.Count -ne $ActualFiles.Count) {
                    throw "scenario $ScenarioID runtime snapshot file count changed after collection"
                }
                foreach ($ActualFile in $ActualFiles) {
                    $Matches = @($DeclaredFiles | Where-Object {
                        [string]$_.relativeFile -ceq [string]$ActualFile.relativeFile
                    })
                    if ($Matches.Count -ne 1 -or
                        [int64]$Matches[0].sizeBytes -ne [int64]$ActualFile.sizeBytes -or
                        [string]$Matches[0].sha256 -cne [string]$ActualFile.sha256) {
                        throw "scenario $ScenarioID runtime snapshot bytes changed after collection"
                    }
                }
                $SnapshotLog = Join-Path $SnapshotRoot "scenarios.jsonl"
                $LogContract = Assert-ProbeEvidenceJSONL -Path $SnapshotLog
                if ([int]$Snapshot.eventCount -ne [int]$LogContract.EventCount -or
                    [string]$Snapshot.firstEventUTC -cne [string]$LogContract.FirstTimestampUTC -or
                    [string]$Snapshot.lastEventUTC -cne [string]$LogContract.LastTimestampUTC) {
                    throw "scenario $ScenarioID runtime snapshot log contract changed after collection"
                }
            }
        }
    }
}

function ConvertTo-EvidenceMarkdownCell {
    param([AllowNull()]$Value)

    if ($null -eq $Value) { return "" }
    $Text = [string]$Value
    $Text = $Text.Replace('&', '&amp;')
    $Text = $Text.Replace('<', '&lt;')
    $Text = $Text.Replace('>', '&gt;')
    $Text = $Text.Replace('|', '\|')
    $Text = $Text.Replace("`r`n", '<br>')
    $Text = $Text.Replace("`n", '<br>')
    $Text.Replace("`r", '<br>')
}

function Get-EvidenceMatrixMarkdown {
    param([Parameter(Mandatory = $true)]$State)

    $Lines = @(
        "# Windows hardware evidence matrix",
        "",
        "> Operator verdicts are unreviewed and do not imply task acceptance.",
        "",
        "- Run: $(ConvertTo-EvidenceMarkdownCell $State.Run.runId)",
        "- OS family: $(ConvertTo-EvidenceMarkdownCell $State.Run.osFamily)",
        "- Package SHA-256: $(ConvertTo-EvidenceMarkdownCell $State.Run.packageSha256)",
        "",
        "| ID | Scenario | Verdict | Review | Recorded UTC | Evidence | Observation | Next action |",
        "| --- | --- | --- | --- | --- | --- | --- | --- |"
    )
    foreach ($Row in @($State.Matrix.scenarios)) {
        $Evidence = @(
            $Row.evidence | ForEach-Object {
                $Reference = "$(ConvertTo-EvidenceMarkdownCell $_.kind):$(ConvertTo-EvidenceMarkdownCell $_.relativeFile)@$(ConvertTo-EvidenceMarkdownCell $_.sha256)"
                $Reference
            }
        ) -join '<br>'
        $Lines += "| $(ConvertTo-EvidenceMarkdownCell $Row.id) | $(ConvertTo-EvidenceMarkdownCell $Row.title) | $(ConvertTo-EvidenceMarkdownCell $Row.verdict) | $(ConvertTo-EvidenceMarkdownCell $Row.reviewState) | $(ConvertTo-EvidenceMarkdownCell $Row.recordedAtUTC) | $Evidence | $(ConvertTo-EvidenceMarkdownCell $Row.observation) | $(ConvertTo-EvidenceMarkdownCell $Row.nextAction) |"
    }
    [string]::Join([Environment]::NewLine, $Lines) + [Environment]::NewLine
}

function Seal-EvidenceRun {
    $State = Get-EvidenceRun
    if ((Test-Path -LiteralPath (Join-Path $State.Root "sealed.json")) -or
        (Test-Path -LiteralPath (Join-Path $State.Root "bundle-manifest.json"))) {
        throw "evidence run is already sealed"
    }
    foreach ($ScenarioID in $script:ProbeEvidenceScenarios) {
        $Row = Get-MatrixRow -Matrix $State.Matrix -ScenarioID $ScenarioID
        if ([string]$Row.verdict -cnotin @("PASS", "FAIL", "BLOCKED")) {
            throw "scenario $ScenarioID lacks an honest terminal operator verdict"
        }
        if (@($Row.evidence).Count -eq 0) {
            throw "scenario $ScenarioID lacks evidence references"
        }
        if ([string]$Row.collectionState -cne "collected-unreviewed" -or
            [string]$Row.reviewState -cne "unreviewed" -or
            [string]::IsNullOrWhiteSpace([string]$Row.observation)) {
            throw "scenario $ScenarioID has an invalid collection, review, or observation state"
        }
        $RecordedAt = [DateTimeOffset]::MinValue
        if (-not [DateTimeOffset]::TryParse([string]$Row.recordedAtUTC, [ref]$RecordedAt)) {
            throw "scenario $ScenarioID has an invalid terminal timestamp"
        }
        if ([string]$Row.verdict -ceq "PASS" -and
            -not [string]::IsNullOrWhiteSpace([string]$Row.nextAction)) {
            throw "scenario $ScenarioID is PASS but carries a failure next action"
        }
        if ([string]$Row.verdict -cne "PASS" -and [string]::IsNullOrWhiteSpace([string]$Row.nextAction)) {
            throw "scenario $ScenarioID is non-PASS without a next action"
        }
    }
    Assert-EvidenceReferencesIntact -State $State

    $CleanupAccepted = $false
    $H17 = Get-MatrixRow -Matrix $State.Matrix -ScenarioID "H17"
    foreach ($Reference in @($H17.evidence)) {
        $Relative = [string]$Reference.relativeFile
        if ($Relative -notmatch '^attachments/H17/[A-Za-z0-9][A-Za-z0-9._-]{0,127}\.json$') { continue }
        $Candidate = Join-Path $State.Root $Relative.Replace('/', [IO.Path]::DirectorySeparatorChar)
        if (-not (Test-Path -LiteralPath $Candidate -PathType Leaf)) { continue }
        $Cleanup = Get-Content -LiteralPath $Candidate -Raw | ConvertFrom-Json
        if ([int]$Cleanup.schemaVersion -eq 1 -and
            [string]$Cleanup.verificationBoundary -ceq "post-evidence-cleanup-only; not hardware scenario acceptance" -and
            [string]$Cleanup.packageSha256 -ceq [string]$State.Run.packageSha256 -and
            [string]$Cleanup.packageIdentity -ceq [string]$State.Run.packageIdentity -and
            [string]$Cleanup.packageFamilyName -ceq [string]$State.Run.packageFamilyName -and
            [bool]$Cleanup.processAbsent -and
            [bool]$Cleanup.packageAbsent -and
            [bool]$Cleanup.signerTrustAbsent -and
            [bool]$Cleanup.runtimeRootAbsent -and
            [bool]$Cleanup.hotkeyAvailable -and
            [bool]$Cleanup.pickerFixtureExclusiveAccess -and
            [bool]$Cleanup.pickerFixtureDeleted -and
            [int]$Cleanup.remainingPartialCount -eq 0) {
            $CleanupAccepted = $true
            break
        }
    }
    if (-not $CleanupAccepted) {
        throw "H17 requires a valid uninstall-probe cleanup receipt"
    }

    foreach ($File in @(Get-ChildItem -LiteralPath $State.Root -Recurse -File -Force)) {
        Assert-ProbeEvidenceFileName -Name $File.Name | Out-Null
        $Extension = $File.Extension.ToLowerInvariant()
        if ($Extension -ceq ".jsonl") {
            Assert-ProbeEvidenceJSONL -Path $File.FullName | Out-Null
        } elseif ($Extension -ceq ".json") {
            $Parsed = Get-Content -LiteralPath $File.FullName -Raw | ConvertFrom-Json
            Assert-ProbeEvidenceValueSafe -Value $Parsed
        } elseif ($Extension -cin @(".csv", ".log", ".md", ".txt", ".xml")) {
            Assert-ProbeEvidenceValueSafe -Value (Get-Content -LiteralPath $File.FullName -Raw) -PropertyName "bundleText"
        }
    }

    Get-ProbeEvidenceFileManifest -Root $State.Root | Out-Null
    $MatrixMarkdown = Get-EvidenceMatrixMarkdown -State $State
    Assert-ProbeEvidenceValueSafe -Value $MatrixMarkdown -PropertyName "matrixText"
    Write-ProbeEvidenceText -Value $MatrixMarkdown -Path (Join-Path $State.Root "matrix.md")

    $Files = Get-ProbeEvidenceFileManifest -Root $State.Root
    $BundleManifest = [ordered]@{
        schemaVersion = 1
        verificationBoundary = "hash-index-before-seal; excludes this manifest and sealed receipt"
        runId = [string]$State.Run.runId
        packageSha256 = [string]$State.Run.packageSha256
        generatedAtUTC = [DateTime]::UtcNow.ToString("o")
        files = @($Files)
    }
    $ManifestPath = Join-Path $State.Root "bundle-manifest.json"
    Write-ProbeEvidenceJSON -Value $BundleManifest -Path $ManifestPath
    $Verdicts = @($State.Matrix.scenarios | ForEach-Object { [string]$_.verdict })
    $Overall = if ($Verdicts -ccontains "BLOCKED") {
        "blocked"
    } elseif ($Verdicts -ccontains "FAIL") {
        "failed"
    } else {
        "all-operator-pass-unreviewed"
    }
    $Seal = [ordered]@{
        schemaVersion = 1
        verificationBoundary = "sealed-operator-evidence; verdicts remain unreviewed and are not task acceptance"
        runId = [string]$State.Run.runId
        osFamily = [string]$State.Run.osFamily
        packageSha256 = [string]$State.Run.packageSha256
        sealedAtUTC = [DateTime]::UtcNow.ToString("o")
        bundleManifestSHA256 = (Get-FileHash -LiteralPath $ManifestPath -Algorithm SHA256).Hash.ToLowerInvariant()
        overallOperatorVerdict = $Overall
        scenarioCount = @($State.Matrix.scenarios).Count
    }
    Write-ProbeEvidenceJSON -Value $Seal -Path (Join-Path $State.Root "sealed.json")
    $Seal
}

switch ($Mode) {
    "Initialize" { Initialize-EvidenceRun }
    "Snapshot" { Save-EvidenceSnapshot }
    "Attach" { Add-EvidenceAttachment }
    "Verdict" { Record-EvidenceVerdict }
    "Seal" { Seal-EvidenceRun }
}
