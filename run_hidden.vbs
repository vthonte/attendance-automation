Set objShell = CreateObject("WScript.Shell")
Set objFSO = CreateObject("Scripting.FileSystemObject")
baseDir = objFSO.GetParentFolderName(WScript.ScriptFullName)
batPath = objFSO.BuildPath(baseDir, "attendance.bat")
objShell.CurrentDirectory = baseDir
objShell.Run """" & batPath & """", 0, False
