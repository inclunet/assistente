Unicode true

####
## Please note: Template replacements don't work in this file. They are provided with default defines like
## mentioned underneath.
## If the keyword is not defined, "wails_tools.nsh" will populate them with the values from ProjectInfo.
## If they are defined here, "wails_tools.nsh" will not touch them. This allows to use this project.nsi manually
## from outside of Wails for debugging and development of the installer.
##
## For development first make a wails nsis build to populate the "wails_tools.nsh":
## > wails build --target windows/amd64 --nsis
## Then you can call makensis on this file with specifying the path to your binary:
## For a AMD64 only installer:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app.exe
## For a ARM64 only installer:
## > makensis -DARG_WAILS_ARM64_BINARY=..\..\bin\app.exe
## For a installer with both architectures:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app-amd64.exe -DARG_WAILS_ARM64_BINARY=..\..\bin\app-arm64.exe
####
## The following information is taken from the ProjectInfo file, but they can be overwritten here.
####
## !define INFO_PROJECTNAME    "MyProject" # Default "{{.Name}}"
## !define INFO_COMPANYNAME    "MyCompany" # Default "{{.Info.CompanyName}}"
## !define INFO_PRODUCTNAME    "MyProduct" # Default "{{.Info.ProductName}}"
## !define INFO_PRODUCTVERSION "1.0.0"     # Default "{{.Info.ProductVersion}}"
## !define INFO_COPYRIGHT      "Copyright" # Default "{{.Info.Copyright}}"
###
## !define PRODUCT_EXECUTABLE  "Application.exe"      # Default "${INFO_PROJECTNAME}.exe"
## !define UNINST_KEY_NAME     "UninstKeyInRegistry"  # Default "${INFO_COMPANYNAME}${INFO_PRODUCTNAME}"
####
## !define REQUEST_EXECUTION_LEVEL "admin"            # Default "admin"  see also https://nsis.sourceforge.io/Docs/Chapter4.html
####
## Include the wails tools
####
!include "wails_tools.nsh"

# The version information for this two must consist of 4 parts
VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

# Enable HiDPI support. https://nsis.sourceforge.io/Reference/ManifestDPIAware
ManifestDPIAware true

!include "MUI.nsh"
!include "LogicLib.nsh"
!include "FileFunc.nsh"

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
# !define MUI_WELCOMEFINISHPAGE_BITMAP "resources\leftimage.bmp" #Include this to add a bitmap on the left side of the Welcome Page. Must be a size of 164x314
!define MUI_FINISHPAGE_NOAUTOCLOSE # Wait on the INSTFILES page so the user can take a look into the details of the installation steps
!define MUI_FINISHPAGE_RUN # Opção para executar o programa após instalação
!define MUI_FINISHPAGE_RUN_FUNCTION ExecutarComoUsuario
!define MUI_FINISHPAGE_RUN_TEXT "Executar ${INFO_PRODUCTNAME}" # Texto do checkbox
!define MUI_ABORTWARNING # This will warn the user if they exit from the installer.

!insertmacro MUI_PAGE_WELCOME # Welcome to the installer page.
# !insertmacro MUI_PAGE_LICENSE "resources\eula.txt" # Adds a EULA page to the installer
!insertmacro MUI_PAGE_DIRECTORY # In which folder install page.
!insertmacro MUI_PAGE_INSTFILES # Installing page.
!insertmacro MUI_PAGE_FINISH # Finished installation page.

!insertmacro MUI_UNPAGE_INSTFILES # Uinstalling page

!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "PortugueseBR"
!insertmacro MUI_LANGUAGE "Spanish"
LangString UserPathError ${LANG_ENGLISH} "Could not update the user's PATH. Installation was stopped. Details: $1"
LangString UserPathError ${LANG_PORTUGUESEBR} "Não foi possível atualizar o PATH do usuário. A instalação foi interrompida. Detalhes: $1"
LangString UserPathError ${LANG_SPANISH} "No se pudo actualizar el PATH del usuario. La instalación se detuvo. Detalles: $1"

## The following two statements can be used to sign the installer and the uninstaller. The path to the binaries are provided in %1
#!uninstfinalize 'signtool --file "%1"'
#!finalize 'signtool --file "%1"'

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe" # Name of the installer's file.
InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}" # Default installing folder ($PROGRAMFILES is Program Files folder).
ShowInstDetails show # This will always show the installation details.

LangString UserPathCleanupWarning ${LANG_ENGLISH} "The application will be removed, but a PATH entry could not be cleaned. Remove the indicated folder from that account's PATH if needed: $1"
LangString UserPathCleanupWarning ${LANG_PORTUGUESEBR} "O aplicativo será removido, mas uma entrada do PATH não pôde ser limpa. Se necessário, remova a pasta indicada do PATH dessa conta: $1"
LangString UserPathCleanupWarning ${LANG_SPANISH} "La aplicación se eliminará, pero no se pudo limpiar una entrada del PATH. Si es necesario, quite la carpeta indicada del PATH de esa cuenta: $1"

Var SilentMode

Function .onInit
   !insertmacro wails.checkArchitecture
   
   # Detecta se está em modo silencioso
   ${GetParameters} $R0
   ${GetOptions} $R0 "/S" $R1
   IfErrors +2 0
   StrCpy $SilentMode "1"
FunctionEnd

Function ExecutarComoUsuario
   # O instalador roda elevado. Abrir o app por aqui faria dele um processo de
   # administrador, e tudo que ele lança depois (terminais, agentes) herdaria
   # isso. O explorer.exe roda com o token do usuário e devolve o nível normal.
   Exec '"$WINDIR\explorer.exe" "$INSTDIR\${PRODUCT_EXECUTABLE}"'
FunctionEnd

Function .onInstSuccess
   # Se for modo silencioso (atualização automática), reabre o programa
   ${If} $SilentMode == "1"
      Call ExecutarComoUsuario
   ${EndIf}
FunctionEnd

Section
    !insertmacro wails.setShellContext

    !insertmacro wails.webview2runtime

    SetOutPath $INSTDIR
    
    # Em modo silencioso (atualização automática) o app pode continuar de pé por
    # alguns instantes: sobrescrever o executável em uso falharia no meio da
    # cópia dos arquivos.
    ${If} $SilentMode == "1"
    ${AndIf} ${FileExists} "$INSTDIR\${PRODUCT_EXECUTABLE}"
        DetailPrint "Aguardando aplicativo fechar..."

        # Versões antigas do instalador renomeavam o executável para .old e nem
        # sempre conseguiam apagá-lo. O resto que ficava para trás bloqueava
        # todas as atualizações seguintes, então limpe antes de esperar.
        Delete "$INSTDIR\${PRODUCT_EXECUTABLE}.old"

        StrCpy $R0 0
        ${Do}
            # Abrir para escrita é o teste honesto de "ninguém está usando": o
            # Windows deixa até renomear um executável em execução, mas nega
            # escrita enquanto a imagem estiver carregada.
            ClearErrors
            FileOpen $R1 "$INSTDIR\${PRODUCT_EXECUTABLE}" a
            ${IfNot} ${Errors}
                FileClose $R1
                ${ExitDo}
            ${EndIf}

            IntOp $R0 $R0 + 1
            ${If} $R0 > 60  # Desiste depois de ~30 s de espera
                MessageBox MB_ICONEXCLAMATION|MB_OK "O ${INFO_PRODUCTNAME} continua em execução e o instalador não pode substituí-lo. Feche o aplicativo e execute o instalador novamente."
                Abort
            ${EndIf}
            Sleep 500  # Aguarda meio segundo
        ${Loop}

        DetailPrint "Aplicativo fechado, continuando instalação..."
    ${EndIf}

    # Também espera a CLI fechar: não substituir asst.exe enquanto estiver em uso.
    ${If} ${FileExists} "$INSTDIR\asst.exe"
        StrCpy $R0 0
        ${Do}
            ClearErrors
            FileOpen $R1 "$INSTDIR\asst.exe" a
            ${IfNot} ${Errors}
                FileClose $R1
                ${ExitDo}
            ${EndIf}
            IntOp $R0 $R0 + 1
            ${If} $R0 > 60
                StrCpy $1 "asst.exe"
                MessageBox MB_ICONEXCLAMATION|MB_OK "$(UserPathError)"
                Abort
            ${EndIf}
            Sleep 500
        ${Loop}
    ${EndIf}

    !insertmacro wails.files
    !ifdef SUPPORTS_AMD64
        ${If} ${IsNativeAMD64}
            File /oname=asst.exe "..\..\bin\asst-amd64.exe"
        ${EndIf}
    !endif
    !ifdef SUPPORTS_ARM64
        ${If} ${IsNativeARM64}
            File /oname=asst.exe "..\..\bin\asst-arm64.exe"
        ${EndIf}
    !endif

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols

    # Adiciona menu de contexto "Abrir Assistente Aqui" para pastas
    WriteRegStr HKCR "Directory\shell\OpenAssistente" "" "Abrir Assistente Aqui"
    WriteRegStr HKCR "Directory\shell\OpenAssistente" "Icon" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    WriteRegStr HKCR "Directory\shell\OpenAssistente\command" "" '"$INSTDIR\${PRODUCT_EXECUTABLE}" "%V\."'

    # Adiciona menu de contexto para background de pastas (clique em espaço vazio)
    WriteRegStr HKCR "Directory\Background\shell\OpenAssistente" "" "Abrir Assistente Aqui"
    WriteRegStr HKCR "Directory\Background\shell\OpenAssistente" "Icon" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    WriteRegStr HKCR "Directory\Background\shell\OpenAssistente\command" "" '"$INSTDIR\${PRODUCT_EXECUTABLE}" "%V\."'

    !insertmacro wails.writeUninstaller

    # A CLI oferece uma operação interna sem inicializar banco ou serviços.
    nsExec::ExecToStack '"$INSTDIR\asst.exe" _installer-path add'
    Pop $0
    Pop $1
    ${If} $0 != 0
        MessageBox MB_ICONEXCLAMATION|MB_OK "$(UserPathError)"
        Abort
    ${EndIf}
SectionEnd

Section "uninstall"
    # Limpa somente as entradas registradas por esta instalação, antes de apagar a CLI.
    nsExec::ExecToStack '"$INSTDIR\asst.exe" _installer-path remove'
    Pop $0
    Pop $1
    ${If} $0 != 0
        StrCpy $1 "$INSTDIR: $1"
    ${EndIf}
    ${If} $1 != ""
        MessageBox MB_ICONEXCLAMATION|MB_OK "$(UserPathCleanupWarning)"
    ${EndIf}
    !insertmacro wails.setShellContext

    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}" # Remove the WebView2 DataPath

    RMDir /r $INSTDIR

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    # Remove menu de contexto "Abrir Assistente Aqui"
    DeleteRegKey HKCR "Directory\shell\OpenAssistente"
    DeleteRegKey HKCR "Directory\Background\shell\OpenAssistente"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols

    !insertmacro wails.deleteUninstaller
SectionEnd
