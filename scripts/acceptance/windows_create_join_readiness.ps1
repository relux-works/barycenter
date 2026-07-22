[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$OutputPath
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
Add-Type -AssemblyName UIAutomationClient

Add-Type -TypeDefinition @"
using System;
using System.Runtime.InteropServices;
public static class PulsarCreateJoinNative {
    [DllImport("user32.dll")] public static extern bool IsHungAppWindow(IntPtr hWnd);
    [DllImport("user32.dll")] public static extern uint GetDpiForWindow(IntPtr hWnd);
    [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr hWnd);
    [DllImport("user32.dll")] public static extern IntPtr GetDlgItem(IntPtr hWnd, int id);
    [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr hWnd);
    [DllImport("user32.dll")] public static extern bool IsWindowEnabled(IntPtr hWnd);
    [DllImport("user32.dll", EntryPoint = "GetWindowLongPtrW")]
    private static extern IntPtr GetWindowLongPtrW(IntPtr hWnd, int index);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)]
    private static extern int GetClassNameW(IntPtr hWnd, System.Text.StringBuilder text, int length);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)]
    private static extern int GetWindowTextW(IntPtr hWnd, System.Text.StringBuilder text, int length);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)]
    private static extern int GetWindowTextLengthW(IntPtr hWnd);
    [DllImport("user32.dll", SetLastError = true)]
    private static extern IntPtr SendMessageTimeoutW(
        IntPtr hWnd, uint message, UIntPtr wParam, IntPtr lParam,
        uint flags, uint timeout, out UIntPtr result);

    public static string ClassName(IntPtr hWnd) {
        var text = new System.Text.StringBuilder(256);
        return GetClassNameW(hWnd, text, text.Capacity) > 0 ? text.ToString() : "";
    }
    public static string WindowText(IntPtr hWnd) {
        var text = new System.Text.StringBuilder(GetWindowTextLengthW(hWnd) + 1);
        GetWindowTextW(hWnd, text, text.Capacity);
        return text.ToString();
    }
    public static bool HasStyle(IntPtr hWnd, long style) {
        return (GetWindowLongPtrW(hWnd, -16).ToInt64() & style) == style;
    }
    public static bool ClickButton(IntPtr hWnd, uint timeout) {
        UIntPtr result;
        return SendMessageTimeoutW(
            hWnd, 0x00F5, UIntPtr.Zero, IntPtr.Zero, 0x0002, timeout, out result
        ) != IntPtr.Zero;
    }
}
"@

$ExpectedPackageFullName = 'ReluxWorksLLC.PulsarBarycenter_0.1.20.0_x64__q036g2bzd7ngc'
$ExpectedHashes = [ordered]@{
    'pulsar-win-amd64.exe' = '0a77f53f026b77dd6abc3b265f18a8d32744847ca23571e97ddd999cc17a0042'
    'go-librespot.exe' = '1967b76fc6e8e91763cea10c1cac1bb5f97cdb08a6100bdb27c9a01470cf84ca'
    'pulsar-capture.dll' = '8c1657d035ab738559c91c4c8468d6a4ba663a80dc96aab8951cc4c2d3b52c2f'
}

function Get-PulsarProcess {
    Get-Process 'pulsar-win-amd64' -ErrorAction SilentlyContinue |
        Where-Object {
            $_.SessionId -eq (Get-Process -Id $PID).SessionId -and
            $_.MainWindowHandle -ne [IntPtr]::Zero
        } |
        Select-Object -First 1
}

function Get-AutomationElement {
    param(
        [Parameter(Mandatory = $true)]
        [Windows.Automation.AutomationElement]$Root,
        [Parameter(Mandatory = $true)]
        [string]$AutomationId
    )
    $condition = New-Object Windows.Automation.PropertyCondition(
        [Windows.Automation.AutomationElement]::AutomationIdProperty,
        $AutomationId
    )
    $matches = $Root.FindAll([Windows.Automation.TreeScope]::Descendants, $condition)
    @($matches | Where-Object { -not $_.Current.IsOffscreen }) | Select-Object -First 1
}

function Get-CredentialPosture {
    $config = Join-Path $env:APPDATA 'Pulsar'
    [ordered]@{
        configDirectory = $config
        protectedCredentialsPresent = Test-Path -LiteralPath (
            Join-Path $config 'credentials.v1.dpapi'
        )
        legacyCredentialsPresent = Test-Path -LiteralPath (
            Join-Path $config 'credentials.json'
        )
    }
}

function Get-EventCount {
    param(
        [Parameter(Mandatory = $true)]
        [string]$LogName,
        [Parameter(Mandatory = $true)]
        [int]$Id,
        [Parameter(Mandatory = $true)]
        [DateTime]$StartTime,
        [Parameter(Mandatory = $true)]
        [string]$Pattern
    )
    @(
        Get-WinEvent -FilterHashtable @{
            LogName = $LogName
            Id = $Id
            StartTime = $StartTime
        } -ErrorAction SilentlyContinue |
            Where-Object Message -Like $Pattern
    ).Count
}

$errorPath = "$OutputPath.error.txt"
Remove-Item -LiteralPath $OutputPath -Force -ErrorAction SilentlyContinue
Remove-Item -LiteralPath $errorPath -Force -ErrorAction SilentlyContinue

try {
    $package = Get-AppxPackage -Name 'ReluxWorksLLC.PulsarBarycenter' -ErrorAction Stop
    if ($package.PackageFullName -ne $ExpectedPackageFullName) {
        throw "unexpected package $($package.PackageFullName)"
    }
    if ($package.Status.ToString() -ne 'Ok') { throw "package status is $($package.Status)" }
    if ($package.SignatureKind.ToString() -ne 'Developer') {
        throw "signature kind is $($package.SignatureKind)"
    }

    $componentHashes = [ordered]@{}
    foreach ($name in $ExpectedHashes.Keys) {
        $path = Join-Path $package.InstallLocation $name
        if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
            throw "installed component is missing: $name"
        }
        $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $path).Hash.ToLowerInvariant()
        if ($actual -ne $ExpectedHashes[$name]) {
            throw "installed component hash drift: $name"
        }
        $componentHashes[$name] = $actual
    }

    $credentialBefore = Get-CredentialPosture
    if ($credentialBefore.protectedCredentialsPresent -or $credentialBefore.legacyCredentialsPresent) {
        throw 'Windows candidate is already paired'
    }

    $configDir = Join-Path $env:APPDATA 'Pulsar'
    $crashPath = Join-Path $configDir 'pulsar-crash.log'
    $crashBytesBefore = if (Test-Path -LiteralPath $crashPath) {
        (Get-Item -LiteralPath $crashPath).Length
    } else { 0 }

    Get-Process 'pulsar-win-amd64' -ErrorAction SilentlyContinue | Stop-Process -Force
    $shortcut = Join-Path ([Environment]::GetFolderPath('Desktop')) 'Pulsar.lnk'
    if (-not (Test-Path -LiteralPath $shortcut -PathType Leaf)) {
        throw 'Pulsar desktop shortcut is missing'
    }

    $startedAt = [DateTime]::UtcNow
    $shell = New-Object -ComObject Shell.Application
    $shortcutItem = $shell.Namespace(0).ParseName('Pulsar.lnk')
    if ($null -eq $shortcutItem) { throw 'Desktop Shell cannot resolve Pulsar.lnk' }
    $shortcutItem.InvokeVerb()

    $deadline = [DateTime]::UtcNow.AddSeconds(20)
    $process = $null
    do {
        Start-Sleep -Milliseconds 200
        $process = Get-PulsarProcess
        if ($null -ne $process) { $process.Refresh() }
    } while (
        ($null -eq $process -or $process.MainWindowHandle -eq [IntPtr]::Zero) -and
        [DateTime]::UtcNow -lt $deadline
    )
    if ($null -eq $process -or $process.MainWindowHandle -eq [IntPtr]::Zero) {
        throw 'Pulsar did not expose an interactive product window'
    }
    [PulsarCreateJoinNative]::SetForegroundWindow($process.MainWindowHandle) | Out-Null

    $condition = New-Object Windows.Automation.PropertyCondition(
        [Windows.Automation.AutomationElement]::ProcessIdProperty,
        $process.Id
    )
    $root = [Windows.Automation.AutomationElement]::RootElement.FindFirst(
        [Windows.Automation.TreeScope]::Children,
        $condition
    )
    if ($null -eq $root -or $root.Current.IsOffscreen) {
        throw 'Pulsar UI Automation root is unavailable or offscreen'
    }

    $joinNavigation = Get-AutomationElement -Root $root -AutomationId '3003'
    if ($null -eq $joinNavigation) {
        $elements = $root.FindAll(
            [Windows.Automation.TreeScope]::Descendants,
            [Windows.Automation.Condition]::TrueCondition
        )
        $diagnostic = @(
            $elements | ForEach-Object {
                [ordered]@{
                    automationId = $_.Current.AutomationId
                    name = $_.Current.Name
                    controlType = $_.Current.ControlType.ProgrammaticName
                    enabled = $_.Current.IsEnabled
                    offscreen = $_.Current.IsOffscreen
                }
            }
        )
        throw (
            'visible Join navigation button was not found; root=' +
            $root.Current.Name + '; elements=' +
            ($diagnostic | ConvertTo-Json -Depth 4 -Compress)
        )
    }
    $joinNavigationName = $joinNavigation.Current.Name
    if ([string]::IsNullOrWhiteSpace($joinNavigationName)) {
        throw 'Join navigation label is empty'
    }
    $joinNavigationHandle = [PulsarCreateJoinNative]::GetDlgItem(
        $process.MainWindowHandle,
        3003
    )
    if ($joinNavigationHandle -eq [IntPtr]::Zero) {
        throw 'native Join navigation control was not found'
    }
    if ([PulsarCreateJoinNative]::ClassName($joinNavigationHandle) -ne 'Button') {
        throw 'native Join navigation control is not a Button'
    }
    if (-not [PulsarCreateJoinNative]::IsWindowVisible($joinNavigationHandle) -or
        -not [PulsarCreateJoinNative]::IsWindowEnabled($joinNavigationHandle)) {
        throw 'native Join navigation control is unavailable'
    }
    if (-not [PulsarCreateJoinNative]::ClickButton($joinNavigationHandle, 2000)) {
        throw 'native Join navigation click timed out'
    }
    Start-Sleep -Milliseconds 250

    $process.Refresh()
    if ($process.HasExited -or -not $process.Responding) {
        throw 'Pulsar stopped responding after opening Join'
    }
    if ([PulsarCreateJoinNative]::IsHungAppWindow($process.MainWindowHandle)) {
        throw 'Pulsar is hung after opening Join'
    }

    $joinInput = Get-AutomationElement -Root $root -AutomationId '3027'
    $joinAction = Get-AutomationElement -Root $root -AutomationId '3010'
    if ($null -eq $joinInput) { throw 'visible invitation input was not found' }
    if ($null -eq $joinAction) { throw 'visible Join action was not found' }
    if (-not $joinInput.Current.IsEnabled) { throw 'invitation input is disabled' }
    if (-not $joinAction.Current.IsEnabled) { throw 'Join action is disabled' }
    $inputPattern = $null
    $inputPatternAvailable = $joinInput.TryGetCurrentPattern(
        [Windows.Automation.ValuePattern]::Pattern,
        [ref]$inputPattern
    )
    $actionPattern = $null
    $actionPatternAvailable = $joinAction.TryGetCurrentPattern(
        [Windows.Automation.InvokePattern]::Pattern,
        [ref]$actionPattern
    )
    $joinActionName = $joinAction.Current.Name
    if ([string]::IsNullOrWhiteSpace($joinActionName)) { throw 'Join action label is empty' }

    $joinInputHandle = [PulsarCreateJoinNative]::GetDlgItem($process.MainWindowHandle, 3027)
    $joinActionHandle = [PulsarCreateJoinNative]::GetDlgItem($process.MainWindowHandle, 3010)
    if ($joinInputHandle -eq [IntPtr]::Zero -or $joinActionHandle -eq [IntPtr]::Zero) {
        throw 'native Join input or action control was not found'
    }
    if ([PulsarCreateJoinNative]::ClassName($joinInputHandle) -ne 'Edit') {
        throw 'native invitation input is not an Edit control'
    }
    if ([PulsarCreateJoinNative]::ClassName($joinActionHandle) -ne 'Button') {
        throw 'native Join action is not a Button control'
    }
    if (-not [PulsarCreateJoinNative]::IsWindowVisible($joinInputHandle) -or
        -not [PulsarCreateJoinNative]::IsWindowEnabled($joinInputHandle)) {
        throw 'native invitation input is unavailable'
    }
    if (-not [PulsarCreateJoinNative]::HasStyle($joinInputHandle, 0x00010000)) {
        throw 'native invitation input has no WS_TABSTOP keyboard style'
    }
    if (-not [PulsarCreateJoinNative]::IsWindowVisible($joinActionHandle) -or
        -not [PulsarCreateJoinNative]::IsWindowEnabled($joinActionHandle)) {
        throw 'native Join action is unavailable'
    }
    if ([string]::IsNullOrWhiteSpace(
        [PulsarCreateJoinNative]::WindowText($joinActionHandle)
    )) {
        throw 'native Join action label is empty'
    }

    $credentialAfter = Get-CredentialPosture
    if ($credentialAfter.protectedCredentialsPresent -or $credentialAfter.legacyCredentialsPresent) {
        throw 'readiness inspection unexpectedly created credentials'
    }
    $crashBytesAfter = if (Test-Path -LiteralPath $crashPath) {
        (Get-Item -LiteralPath $crashPath).Length
    } else { 0 }
    if ($crashBytesAfter -gt $crashBytesBefore) {
        throw 'Pulsar wrote a new callback crash during Join inspection'
    }

    $cim = Get-CimInstance Win32_Process | Where-Object ProcessId -eq $process.Id
    $parent = Get-CimInstance Win32_Process | Where-Object ProcessId -eq $cim.ParentProcessId
    if ($parent.Name -ne 'explorer.exe') { throw "unexpected parent process $($parent.Name)" }

    $bounds = $root.Current.BoundingRectangle
    $result = [ordered]@{
        schemaVersion = 1
        task = 'TASK-260722-1zv67l'
        observedAtUTC = [DateTime]::UtcNow.ToString('o')
        host = $env:COMPUTERNAME
        osBuild = [System.Environment]::OSVersion.Version.ToString()
        sourceCommit = '76f09a4d8be693d57cd5d47b9b9e5ac06196519c'
        hostedCIRun = 29863591495
        packageFullName = $package.PackageFullName
        packageFamilyName = $package.PackageFamilyName
        version = $package.Version.ToString()
        packageStatus = $package.Status.ToString()
        signatureKind = $package.SignatureKind.ToString()
        packageArchiveSha256 = 'f74b5c8d6f8c86443f8c1b64715977be1b0183c39e7fc4dde7567c957b958348'
        packageArchiveRehashedNow = $false
        componentHashes = $componentHashes
        installedComponentsRehashedNow = $true
        unpaired = $credentialAfter
        ordinaryLaunch = [ordered]@{
            desktopShortcut = $true
            processId = $process.Id
            parentProcess = $parent.Name
            sessionId = $process.SessionId
            responding = $process.Responding
            hung = [PulsarCreateJoinNative]::IsHungAppWindow($process.MainWindowHandle)
            dpi = [PulsarCreateJoinNative]::GetDpiForWindow($process.MainWindowHandle)
            windowVisible = -not $root.Current.IsOffscreen
            windowWidth = [Math]::Round($bounds.Width)
            windowHeight = [Math]::Round($bounds.Height)
            crashLogBytesBefore = $crashBytesBefore
            crashLogBytesAfter = $crashBytesAfter
            applicationCrashEvents = Get-EventCount -LogName 'Application' -Id 1000 `
                -StartTime $startedAt -Pattern '*pulsar-win-amd64*'
            appModelRemovalEvents = Get-EventCount `
                -LogName 'Microsoft-Windows-AppModel-Runtime/Admin' -Id 217 `
                -StartTime $startedAt -Pattern "*$($package.PackageFamilyName)*"
        }
        joinSurface = [ordered]@{
            navigationAutomationId = '3003'
            navigationName = $joinNavigationName
            navigationControlType = $joinNavigation.Current.ControlType.ProgrammaticName
            navigationInvoked = $true
            navigationNativeClass = [PulsarCreateJoinNative]::ClassName(
                $joinNavigationHandle
            )
            navigationNativeClickCompleted = $true
            inputAutomationId = '3027'
            inputName = $joinInput.Current.Name
            inputControlType = $joinInput.Current.ControlType.ProgrammaticName
            inputVisible = -not $joinInput.Current.IsOffscreen
            inputEnabled = $joinInput.Current.IsEnabled
            inputKeyboardFocusable = $joinInput.Current.IsKeyboardFocusable
            inputValuePatternAvailable = $inputPatternAvailable
            inputNativeClass = [PulsarCreateJoinNative]::ClassName($joinInputHandle)
            inputNativeVisible = [PulsarCreateJoinNative]::IsWindowVisible($joinInputHandle)
            inputNativeEnabled = [PulsarCreateJoinNative]::IsWindowEnabled($joinInputHandle)
            inputNativeTabStop = [PulsarCreateJoinNative]::HasStyle(
                $joinInputHandle,
                0x00010000
            )
            actionAutomationId = '3010'
            actionName = $joinActionName
            actionControlType = $joinAction.Current.ControlType.ProgrammaticName
            actionVisible = -not $joinAction.Current.IsOffscreen
            actionEnabled = $joinAction.Current.IsEnabled
            actionInvokePatternAvailable = $actionPatternAvailable
            actionNativeClass = [PulsarCreateJoinNative]::ClassName($joinActionHandle)
            actionNativeVisible = [PulsarCreateJoinNative]::IsWindowVisible($joinActionHandle)
            actionNativeEnabled = [PulsarCreateJoinNative]::IsWindowEnabled($joinActionHandle)
            uiaSemanticStatus = 'unexpected-pane-no-patterns'
            invitationEntered = $false
            joinActionInvoked = $false
        }
        manualEvidence = 'not-run'
        manualPassClaimed = $false
    }
    $directory = Split-Path -Parent $OutputPath
    if (-not (Test-Path -LiteralPath $directory)) {
        New-Item -ItemType Directory -Path $directory -Force | Out-Null
    }
    [IO.File]::WriteAllText(
        $OutputPath,
        ($result | ConvertTo-Json -Depth 8) + [Environment]::NewLine,
        [Text.UTF8Encoding]::new($false)
    )
} catch {
    $directory = Split-Path -Parent $errorPath
    if (-not (Test-Path -LiteralPath $directory)) {
        New-Item -ItemType Directory -Path $directory -Force | Out-Null
    }
    [IO.File]::WriteAllText(
        $errorPath,
        ($_.Exception.ToString() + [Environment]::NewLine + $_.ScriptStackTrace),
        [Text.UTF8Encoding]::new($false)
    )
    throw
}
