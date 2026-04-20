#!/bin/sh
# Instalador do Assistente CLI (asst)
# Uso: curl -sSL https://raw.githubusercontent.com/inclunet/assistente/main/install.sh | sh
#
# Variáveis de ambiente opcionais:
#   INSTALL_DIR  - diretório de instalação (padrão: /usr/local/bin ou ~/.local/bin)
#   VERSION      - versão específica (padrão: latest)

set -e

REPO="inclunet/assistente"
BINARY="asst"

# Cores (desabilitadas se não for terminal)
if [ -t 1 ]; then
    GREEN='\033[0;32m'
    RED='\033[0;31m'
    YELLOW='\033[0;33m'
    NC='\033[0m'
else
    GREEN='' RED='' YELLOW='' NC=''
fi

info()  { printf "${GREEN}✓${NC} %s\n" "$1"; }
warn()  { printf "${YELLOW}!${NC} %s\n" "$1"; }
error() { printf "${RED}✗${NC} %s\n" "$1" >&2; exit 1; }

# Detectar OS
detect_os() {
    case "$(uname -s)" in
        Linux*)  echo "linux" ;;
        Darwin*) echo "darwin" ;;
        MINGW*|MSYS*|CYGWIN*) echo "windows" ;;
        *) error "Sistema operacional não suportado: $(uname -s)" ;;
    esac
}

# Detectar arquitetura
detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)  echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        *) error "Arquitetura não suportada: $(uname -m)" ;;
    esac
}

# Obter versão latest da API do GitHub
get_latest_version() {
    if command -v curl > /dev/null 2>&1; then
        curl -sSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/'
    elif command -v wget > /dev/null 2>&1; then
        wget -qO- "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/'
    else
        error "curl ou wget é necessário para download"
    fi
}

# Download de arquivo
download() {
    url="$1"
    dest="$2"
    if command -v curl > /dev/null 2>&1; then
        curl -sSL -o "$dest" "$url"
    elif command -v wget > /dev/null 2>&1; then
        wget -qO "$dest" "$url"
    fi
}

# Escolher diretório de instalação
choose_install_dir() {
    if [ -n "$INSTALL_DIR" ]; then
        echo "$INSTALL_DIR"
        return
    fi

    # Se pode escrever em /usr/local/bin, usar
    if [ -d "/usr/local/bin" ] && [ -w "/usr/local/bin" ]; then
        echo "/usr/local/bin"
        return
    fi

    # Fallback: ~/.local/bin
    local_bin="$HOME/.local/bin"
    mkdir -p "$local_bin"
    echo "$local_bin"
}

main() {
    OS=$(detect_os)
    ARCH=$(detect_arch)

    if [ "$OS" = "windows" ]; then
        warn "No Windows, use o instalador PowerShell:"
        warn "  irm https://raw.githubusercontent.com/inclunet/assistente/main/install.ps1 | iex"
        exit 1
    fi

    printf "Instalando %s para %s/%s...\n" "$BINARY" "$OS" "$ARCH"

    # Versão
    if [ -n "$VERSION" ]; then
        TAG="$VERSION"
    else
        TAG=$(get_latest_version)
    fi

    if [ -z "$TAG" ]; then
        error "Não foi possível determinar a versão. Defina VERSION=vX.Y.Z"
    fi

    info "Versão: $TAG"

    # Nome do asset na release
    EXT=""
    ASSET_NAME="${BINARY}-${OS}-${ARCH}${EXT}"
    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${TAG}/${ASSET_NAME}"

    # Download
    TMPDIR=$(mktemp -d)
    TMPFILE="${TMPDIR}/${BINARY}"

    printf "Baixando %s... " "$DOWNLOAD_URL"
    download "$DOWNLOAD_URL" "$TMPFILE" || error "Falha no download. Verifique se a versão $TAG existe."
    info "OK"

    # Verificar checksum (se disponível)
    CHECKSUM_URL="${DOWNLOAD_URL}.sha256"
    CHECKSUM_FILE="${TMPDIR}/checksum.sha256"
    if download "$CHECKSUM_URL" "$CHECKSUM_FILE" 2>/dev/null; then
        EXPECTED=$(awk '{print $1}' "$CHECKSUM_FILE")
        if command -v sha256sum > /dev/null 2>&1; then
            ACTUAL=$(sha256sum "$TMPFILE" | awk '{print $1}')
        elif command -v shasum > /dev/null 2>&1; then
            ACTUAL=$(shasum -a 256 "$TMPFILE" | awk '{print $1}')
        fi
        if [ -n "$ACTUAL" ] && [ "$EXPECTED" != "$ACTUAL" ]; then
            error "Checksum inválido! Esperado: $EXPECTED, obtido: $ACTUAL"
        fi
        info "Checksum verificado"
    fi

    chmod +x "$TMPFILE"

    # Instalar
    DEST_DIR=$(choose_install_dir)
    DEST="${DEST_DIR}/${BINARY}"

    if [ -w "$DEST_DIR" ]; then
        mv "$TMPFILE" "$DEST"
    else
        warn "Permissão necessária para instalar em $DEST_DIR"
        sudo mv "$TMPFILE" "$DEST"
    fi

    rm -rf "$TMPDIR"

    info "Instalado em $DEST"

    # Verificar PATH
    if ! echo "$PATH" | tr ':' '\n' | grep -qx "$DEST_DIR"; then
        warn "$DEST_DIR não está no PATH"
        warn "Adicione ao seu shell profile:"
        warn "  export PATH=\"$DEST_DIR:\$PATH\""
    fi

    # Verificar
    if command -v "$BINARY" > /dev/null 2>&1; then
        info "$($BINARY version)"
        printf "\nUso:\n"
        printf "  %s setup          # Configuração inicial\n" "$BINARY"
        printf "  %s chat \"olá\"     # Conversar\n" "$BINARY"
        printf "  %s --help         # Ajuda completa\n" "$BINARY"
    fi
}

main
