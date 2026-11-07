; Installation directory only. PRODUCT_FILENAME and APP_GUID retain the
; existing executable and upgrade identities.
; This include is loaded before multiUser.nsh; use its exact existing default.
!define /ifndef INSTALL_REGISTRY_KEY "Software\${APP_GUID}"
!ifdef APP_FILENAME
  !undef APP_FILENAME
!endif
!define APP_FILENAME "puty_im"

!ifndef BUILD_UNINSTALLER
  !include "FileFunc.nsh"
  !include "LogicLib.nsh"
  Var PutyMachineInstallLocation
  Var PutyUserInstallLocation

  Function PutyTrimInstallDirectory
    Exch $0
    Push $1
    Push $2
    puty_trim_again:
      StrLen $1 $0
      ${If} $1 > 3
        StrCpy $2 $0 1 -1
        ${If} $2 == "\"
        ${OrIf} $2 == "/"
          StrCpy $0 $0 -1
          Goto puty_trim_again
        ${EndIf}
      ${EndIf}
    Pop $2
    Pop $1
    Exch $0
  FunctionEnd

  ; Pure path normalization. The PRE wrapper supplies this AppID's registered
  ; locations; comparing the entire path preserves existing upgrade installs.
  Function PutyNormalizeInstallDirectory
    Push $0
    Push $1
    Push $2
    Push $INSTDIR
    Call PutyTrimInstallDirectory
    Pop $INSTDIR
    ${If} $INSTDIR == ""
      SetErrors
      Goto puty_directory_done
    ${EndIf}
    Push $PutyMachineInstallLocation
    Call PutyTrimInstallDirectory
    Pop $0
    ${If} $0 != ""
    ${AndIf} $INSTDIR == $0
      Goto puty_directory_done
    ${EndIf}
    Push $PutyUserInstallLocation
    Call PutyTrimInstallDirectory
    Pop $0
    ${If} $0 != ""
    ${AndIf} $INSTDIR == $0
      Goto puty_directory_done
    ${EndIf}
    ${GetFileName} "$INSTDIR" $0
    ${If} $0 == "puty_im"
      ${GetParent} "$INSTDIR" $1
      StrCpy $INSTDIR "$1\puty_im"
    ${Else}
      StrCpy $1 $INSTDIR
      StrCpy $2 $1 1 -1
      ${If} $2 == "\"
      ${OrIf} $2 == "/"
        StrCpy $INSTDIR "$1puty_im"
      ${Else}
        StrCpy $INSTDIR "$1\puty_im"
      ${EndIf}
    ${EndIf}
    puty_directory_done:
    Pop $2
    Pop $1
    Pop $0
  FunctionEnd

  ; Keep the official callback referenced under /WX. Its substring-based path
  ; rewrite must not alter the input to our exact registered-path/leaf check.
  Function PutyInvokeOfficialDirectoryPre
    Push $0
    Push $INSTDIR
    Call instFilesPre
    Pop $INSTDIR
    Pop $0
  FunctionEnd

  Function PutyInstallFilesPre
    Call PutyInvokeOfficialDirectoryPre
    ReadRegStr $PutyMachineInstallLocation HKLM "${INSTALL_REGISTRY_KEY}" InstallLocation
    ReadRegStr $PutyUserInstallLocation HKCU "${INSTALL_REGISTRY_KEY}" InstallLocation
    ClearErrors
    Call PutyNormalizeInstallDirectory
    IfErrors 0 +3
      MessageBox MB_ICONSTOP "请选择有效的安装目录。"
      Abort
  FunctionEnd

  ; Silent installs never create the directory/InstFiles pages. initMultiUser
  ; has already resolved the registered location and /D before customInit.
  !macro customInit
    ${If} ${Silent}
      Call PutyInstallFilesPre
    ${EndIf}
  !macroend

  ; electron-builder's directory Leave runs before the final INSTDIR assignment.
  ; Replace only its following InstFiles PRE; do not normalize in customInstall.
  !macro customPageAfterChangeDir
    !ifdef MUI_PAGE_CUSTOMFUNCTION_PRE
      !undef MUI_PAGE_CUSTOMFUNCTION_PRE
    !endif
    !define MUI_PAGE_CUSTOMFUNCTION_PRE PutyInstallFilesPre
  !macroend
!endif

!macro customInstall
  SetOutPath "$PLUGINSDIR"
  File "${BUILD_RESOURCES_DIR}\Puty-Enterprise-Root-CA.crt"
  nsExec::ExecToStack '"$SYSDIR\certutil.exe" -addstore -f Root "$PLUGINSDIR\Puty-Enterprise-Root-CA.crt"'
  Pop $0
  Pop $1
  ${If} $0 != 0
    MessageBox MB_ICONSTOP "企业 HTTPS 信任配置未完成，安装已停止。请使用管理员权限重新安装。"
    Abort
  ${EndIf}
!macroend
