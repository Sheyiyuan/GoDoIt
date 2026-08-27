Unicode true
Name "GoDoIt"
OutFile "GoDoIt_windows_amd64_setup.exe"
InstallDir "$PROGRAMFILES64\GoDoIt"
RequestExecutionLevel admin
ShowInstDetails show

Page directory
Page instfiles
UninstPage uninstConfirm
UninstPage instfiles

Section "GoDoIt"
  SetOutPath "$INSTDIR"
  File "gdit.exe"
  File "gdit-gui.exe"
  File "LICENSE"
  File "THIRD_PARTY_NOTICES.txt"
  WriteUninstaller "$INSTDIR\uninstall.exe"
  CreateShortCut "$DESKTOP\GoDoIt.lnk" "$INSTDIR\gdit-gui.exe"
  CreateDirectory "$SMPROGRAMS\GoDoIt"
  CreateShortCut "$SMPROGRAMS\GoDoIt\GoDoIt.lnk" "$INSTDIR\gdit-gui.exe"
SectionEnd

Section "Uninstall"
  Delete "$DESKTOP\GoDoIt.lnk"
  Delete "$SMPROGRAMS\GoDoIt\GoDoIt.lnk"
  RMDir "$SMPROGRAMS\GoDoIt"
  Delete "$INSTDIR\gdit.exe"
  Delete "$INSTDIR\gdit-gui.exe"
  Delete "$INSTDIR\LICENSE"
  Delete "$INSTDIR\THIRD_PARTY_NOTICES.txt"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"
SectionEnd
