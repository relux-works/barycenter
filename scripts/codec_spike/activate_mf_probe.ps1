[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$ApplicationUserModelId,
    [Parameter(Mandatory = $true)][string]$EvidencePath,
    [ValidateRange(0, 7200)][int]$SoakSeconds = 60
)

$ErrorActionPreference = "Stop"

if (-not ("Pulsar.ApplicationActivationManager" -as [type])) {
    Add-Type -TypeDefinition @"
using System;
using System.Runtime.InteropServices;

namespace Pulsar {
    [ComImport]
    [Guid("2E941141-7F97-4756-BA1D-9DECDE894A3D")]
    [InterfaceType(ComInterfaceType.InterfaceIsIUnknown)]
    public interface IApplicationActivationManager {
        int ActivateApplication(
            [MarshalAs(UnmanagedType.LPWStr)] string appUserModelId,
            [MarshalAs(UnmanagedType.LPWStr)] string arguments,
            uint options,
            out uint processId);
        int ActivateForFile(IntPtr appUserModelId, IntPtr itemArray, IntPtr verb, out uint processId);
        int ActivateForProtocol(IntPtr appUserModelId, IntPtr itemArray, out uint processId);
    }

    [ComImport]
    [Guid("45BA127D-10A8-46EA-8AB7-56EA9078943C")]
    public class ApplicationActivationManager { }
}
"@
}

Remove-Item -Force $EvidencePath -ErrorAction SilentlyContinue
$manager = [Pulsar.IApplicationActivationManager][Pulsar.ApplicationActivationManager]::new()
$processId = [uint32]0
$arguments = "--soak-seconds=$SoakSeconds"
$hresult = $manager.ActivateApplication($ApplicationUserModelId, $arguments, 0, [ref]$processId)
if ($hresult -ne 0) {
    throw ("packaged AppContainer activation failed with HRESULT 0x{0:X8}" -f ([uint32]$hresult))
}
$process = [Diagnostics.Process]::GetProcessById([int]$processId)
if (-not $process.WaitForExit(180000)) {
    $process.Kill()
    throw "packaged AppContainer probe did not exit within 180 seconds"
}
$exitCode = $process.ExitCode
if (-not (Test-Path $EvidencePath)) {
    throw "packaged AppContainer probe did not write its LocalState evidence"
}
[pscustomobject]@{
    ProcessId = [int]$processId
    ExitCode = $exitCode
    EvidencePath = $EvidencePath
    Arguments = $arguments
    DebugModeEnabled = $false
    ActivationOptions = "AO_NONE"
}
