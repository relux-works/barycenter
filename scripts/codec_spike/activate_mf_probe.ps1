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

    public sealed class ActivationResult {
        public int HResult { get; set; }
        public uint ProcessId { get; set; }
    }

    public static class PackagedAppActivator {
        public static ActivationResult Activate(string appUserModelId, string arguments) {
            var manager = (IApplicationActivationManager)new ApplicationActivationManager();
            uint processId;
            int hresult = manager.ActivateApplication(appUserModelId, arguments, 0, out processId);
            Marshal.FinalReleaseComObject(manager);
            return new ActivationResult { HResult = hresult, ProcessId = processId };
        }
    }
}
"@
}

Remove-Item -Force $EvidencePath -ErrorAction SilentlyContinue
$arguments = "--soak-seconds=$SoakSeconds"
$activation = [Pulsar.PackagedAppActivator]::Activate($ApplicationUserModelId, $arguments)
if ($activation.HResult -ne 0) {
    throw ("packaged AppContainer activation failed with HRESULT 0x{0:X8}" -f ([uint32]$activation.HResult))
}
$process = [Diagnostics.Process]::GetProcessById([int]$activation.ProcessId)
if (-not $process.WaitForExit(180000)) {
    $process.Kill()
    throw "packaged AppContainer probe did not exit within 180 seconds"
}
$exitCode = $process.ExitCode
if (-not (Test-Path $EvidencePath)) {
    throw "packaged AppContainer probe did not write its LocalState evidence"
}
[pscustomobject]@{
    ProcessId = [int]$activation.ProcessId
    ExitCode = $exitCode
    EvidencePath = $EvidencePath
    Arguments = $arguments
    DebugModeEnabled = $false
    ActivationOptions = "AO_NONE"
}
