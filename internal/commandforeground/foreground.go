// Package commandforeground captura o contexto do programa em primeiro plano.
//
// O snapshot é transitório: a identidade da janela/processo fica opaca e o
// resumo expõe somente os campos allowlisted pelo D14. Nenhuma operação deste
// pacote lê título, URL, texto ou payload da janela.
package commandforeground

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"path"
	"reflect"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	ProviderVersion     = "windows-foreground.v1"
	maxWindowClassRunes = 256
)

var (
	// ErrUnavailable indica que o adapter nativo não existe nesta plataforma.
	ErrUnavailable = errors.New("foreground: adapter nativo indisponível nesta plataforma")
	// ErrUnknown indica que a identidade do foreground não pôde ser obtida
	// consistentemente. O chamador deve tratar o contexto como desconhecido.
	ErrUnknown = errors.New("foreground: identidade da janela em primeiro plano desconhecida")
	// ErrInvalidSnapshot indica que um Reader devolveu uma observação que não
	// pode ser usada para autorizar ou contextualizar uma operação.
	ErrInvalidSnapshot = errors.New("foreground: snapshot inválido")
)

// Reader fornece uma captura autoritativa do foreground no instante da
// chamada. Implementações devem preservar CapturedAt como timestamp de sua
// própria observação, não como timestamp fornecido pelo chamador.
type Reader interface {
	Capture(context.Context) (Snapshot, error)
}

// Identity é a identidade opaca de uma janela e do processo que a possui.
// Seus componentes não são expostos para evitar que consumidores tratem HWND
// ou PID como dados de negócio ou os serializem acidentalmente.
type Identity struct {
	window       uintptr
	process      uint32
	creationTime uint64
}

// Equal compara identidades capturadas pelo mesmo adapter.
func (i Identity) Equal(other Identity) bool {
	return i == other
}

// IsZero informa se nenhuma identidade válida foi capturada.
func (i Identity) IsZero() bool {
	return i == Identity{}
}

// Summary é a projeção segura do snapshot para auditoria e diagnóstico.
// Estes são deliberadamente os únicos campos de apresentação disponíveis.
type Summary struct {
	Executable      string `json:"executable"`
	WindowClass     string `json:"window_class"`
	ProviderVersion string `json:"provider_version"`
}

// Snapshot é uma observação transitória do foreground.
//
// Identity não é serializável e Summary não contém caminho completo, título,
// URL, documento ou texto da janela.
type Snapshot struct {
	Identity Identity `json:"-"`
	// Version identifica o fato foreground observado, sem CapturedAt. Ele é
	// estável para a mesma identidade/lifetime e não promete exact_version;
	// foreground é usado pela política event_snapshot do D14.
	Version    string    `json:"version"`
	CapturedAt time.Time `json:"captured_at"`
	Summary    Summary   `json:"summary"`
}

// CaptureBeforeShow captura o foreground antes de executar show. Falha na
// captura é fail-closed: show não é chamado. Uma falha de show conserva e
// devolve o snapshot que já foi capturado; este helper nunca captura de novo
// depois que o foco pode ter mudado.
func CaptureBeforeShow(ctx context.Context, reader Reader, show func() error) (Snapshot, error) {
	if ctx == nil {
		return Snapshot{}, errors.New("foreground: contexto nil")
	}
	if isNilReader(reader) {
		return Snapshot{}, errors.New("foreground: reader nil")
	}
	if show == nil {
		return Snapshot{}, errors.New("foreground: show nil")
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}

	snapshot, err := reader.Capture(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	if snapshot.Identity.IsZero() || strings.TrimSpace(snapshot.Version) == "" || snapshot.CapturedAt.IsZero() {
		return Snapshot{}, ErrInvalidSnapshot
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	if err := show(); err != nil {
		return snapshot, err
	}
	return snapshot, nil
}

// normalizeExecutableBase redige um caminho de processo para seu basename
// normalizado. O caminho completo existe somente durante a captura nativa e
// nunca é colocado em Snapshot ou Summary.
func normalizeExecutableBase(value string) string {
	value = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(value), "\x00"))
	value = strings.ReplaceAll(value, `\`, "/")
	value = strings.TrimRight(value, "/")
	if value == "" {
		return ""
	}
	base := strings.TrimSpace(path.Base(value))
	if base == "." || base == ".." || base == "/" {
		return ""
	}
	return strings.ToLower(base)
}

func normalizeWindowClass(value string) string {
	if !utf8.ValidString(value) {
		return ""
	}
	value = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(value), "\x00"))
	if value == "" || utf8.RuneCountInString(value) > maxWindowClassRunes {
		return ""
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return ""
		}
	}
	return value
}

func foregroundFactVersion(identity Identity, summary Summary) string {
	h := sha256.New()
	writeVersionPart(h, "assistente.commandforeground.foreground_fact.v1")
	writeVersionUint64(h, uint64(identity.window))
	writeVersionUint64(h, uint64(identity.process))
	writeVersionUint64(h, identity.creationTime)
	writeVersionPart(h, summary.Executable)
	writeVersionPart(h, summary.WindowClass)
	writeVersionPart(h, summary.ProviderVersion)
	return hex.EncodeToString(h.Sum(nil))
}

func writeVersionPart(h interface{ Write([]byte) (int, error) }, value string) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = h.Write(length[:])
	_, _ = h.Write([]byte(value))
}

func writeVersionUint64(h interface{ Write([]byte) (int, error) }, value uint64) {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	_, _ = h.Write(encoded[:])
}

func isNilReader(reader Reader) bool {
	if reader == nil {
		return true
	}
	value := reflect.ValueOf(reader)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
