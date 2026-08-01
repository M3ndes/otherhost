Option Explicit

Dim arguments
Dim fileSystem
Dim launcher
Dim localAppData
Dim parameters
Dim powershellPath
Dim scriptPath
Dim stateDirectory
Dim statusPath
Dim workingDirectory
Dim index

Set arguments = WScript.Arguments
Set fileSystem = CreateObject("Scripting.FileSystemObject")
Set launcher = CreateObject("Shell.Application")

localAppData = CreateObject("WScript.Shell").ExpandEnvironmentStrings("%LOCALAPPDATA%")
stateDirectory = fileSystem.BuildPath(localAppData, "devbox-bridge")
statusPath = fileSystem.BuildPath(stateDirectory, "setup.status")

If arguments.Count < 1 Then
    WriteStatus statusPath, "failed", "The setup script path was not provided"
    WScript.Quit 2
End If

scriptPath = arguments.Item(0)
If Not fileSystem.FileExists(scriptPath) Then
    WriteStatus statusPath, "failed", "The setup script does not exist"
    WScript.Quit 3
End If

workingDirectory = fileSystem.GetParentFolderName(scriptPath)
powershellPath = CreateObject("WScript.Shell").ExpandEnvironmentStrings( _
    "%SystemRoot%\System32\WindowsPowerShell\v1.0\powershell.exe")
parameters = "-NoProfile -ExecutionPolicy Bypass -File " & QuoteArgument(scriptPath)

For index = 1 To arguments.Count - 1
    parameters = parameters & " " & QuoteArgument(arguments.Item(index))
Next

WriteStatus statusPath, "elevation-requested", "Windows setup was handed to the elevation broker"
On Error Resume Next
launcher.ShellExecute powershellPath, parameters, workingDirectory, "runas", 1
If Err.Number <> 0 Then
    WriteStatus statusPath, "failed", "Windows elevation was rejected or could not start"
    WScript.Quit 4
End If
On Error GoTo 0

WScript.Quit 0

Function QuoteArgument(ByVal value)
    If InStr(value, Chr(34)) > 0 Then
        Err.Raise 5, "launch-windows-setup", "Setup arguments cannot contain quotation marks"
    End If
    QuoteArgument = Chr(34) & value & Chr(34)
End Function

Sub WriteStatus(ByVal path, ByVal state, ByVal message)
    Dim stream
    On Error Resume Next
    Set stream = fileSystem.OpenTextFile(path, 2, True, 0)
    stream.WriteLine state
    stream.WriteLine message
    stream.Close
    On Error GoTo 0
End Sub
