[CmdletBinding()]
param(
    [ValidateSet("Install", "Status", "Remove")]
    [string]$Mode = "Status",
    [Parameter(Mandatory = $true)][string]$TaskName,
    [string]$ScriptPath = "",
    [string[]]$ScriptArgumentList = @(),
    [int]$ExecutionTimeLimitMinutes = 10
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

function ConvertTo-HiddenTaskArgument {
    param([Parameter(Mandatory = $true)][string]$Value)

    if ($Value.Contains('"')) {
        throw "scheduled-task arguments may not contain a double quote"
    }
    '"' + $Value + '"'
}

function Get-HiddenInteractiveTaskActionContract {
    param(
        [Parameter(Mandatory = $true)][string]$ObserverScriptPath,
        [string[]]$ObserverArgumentList = @()
    )

    $PowerShell = Join-Path $env:SystemRoot "System32\WindowsPowerShell\v1.0\powershell.exe"
    $Arguments = @(
        "-NoLogo",
        "-NoProfile",
        "-NonInteractive",
        "-WindowStyle", "Hidden",
        "-ExecutionPolicy", "Bypass",
        "-File", (ConvertTo-HiddenTaskArgument -Value $ObserverScriptPath)
    ) + @($ObserverArgumentList | ForEach-Object { ConvertTo-HiddenTaskArgument -Value $_ })
    [pscustomobject]@{
        Execute = $PowerShell
        Arguments = $Arguments -join " "
        WindowStyle = "Hidden"
        ConsoleVisible = $false
    }
}

function Invoke-HiddenInteractiveTask {
    if ($Mode -ceq "Remove") {
        Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false -ErrorAction SilentlyContinue
        return [pscustomobject]@{ TaskName = $TaskName; State = "removed" }
    }
    if ($Mode -ceq "Status") {
        $Task = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
        return [pscustomobject]@{
            TaskName = $TaskName
            State = if ($null -eq $Task) { "absent" } else { [string]$Task.State }
        }
    }
    if ([string]::IsNullOrWhiteSpace($ScriptPath)) {
        throw "Install requires -ScriptPath"
    }
    if ($ExecutionTimeLimitMinutes -lt 1 -or $ExecutionTimeLimitMinutes -gt 60) {
        throw "ExecutionTimeLimitMinutes must be between 1 and 60"
    }
    $ResolvedScriptPath = (Resolve-Path -LiteralPath $ScriptPath).Path
    $Contract = Get-HiddenInteractiveTaskActionContract `
        -ObserverScriptPath $ResolvedScriptPath `
        -ObserverArgumentList $ScriptArgumentList
    $Action = New-ScheduledTaskAction -Execute $Contract.Execute -Argument $Contract.Arguments
    $Principal = New-ScheduledTaskPrincipal `
        -UserId ([Security.Principal.WindowsIdentity]::GetCurrent().Name) `
        -LogonType Interactive `
        -RunLevel Highest
    $Settings = New-ScheduledTaskSettingsSet `
        -ExecutionTimeLimit (New-TimeSpan -Minutes $ExecutionTimeLimitMinutes) `
        -AllowStartIfOnBatteries `
        -DontStopIfGoingOnBatteries
    Register-ScheduledTask -TaskName $TaskName -Action $Action -Principal $Principal -Settings $Settings -Force | Out-Null
    Start-ScheduledTask -TaskName $TaskName
    [pscustomobject]@{
        TaskName = $TaskName
        State = "started"
        ObserverConsoleVisible = $Contract.ConsoleVisible
        Action = $Contract
    }
}

if ($MyInvocation.InvocationName -cne ".") {
    Invoke-HiddenInteractiveTask
}
