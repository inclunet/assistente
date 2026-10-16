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

// NewSnapshot converte uma observação consistente do adapter confiável em um
// snapshot transitório. O adapter deve verificar a identidade e a estabilidade
// do foco antes de chamar esta função; dados de UI/configuração não são uma
// observação do SO. Timestamp, versão e redação do caminho são definidos aqui,
// nunca fornecidos pelo acionador. A identidade permanece opaca e não serializada.
func NewSnapshot(window uintptr, process uint32, creationTime uint64, executablePath, windowClass string) (Snapshot, error) {
	identity := Identity{window: window, process: process, creationTime: creationTime}
	summary := Summary{
		Executable:      normalizeExecutableBase(executablePath),
		WindowClass:     normalizeWindowClass(windowClass),
		ProviderVersion: ProviderVersion,
	}
	if window == 0 || process == 0 || creationTime == 0 || !validSummary(summary) {
		return Snapshot{}, ErrUnknown
	}
	return Snapshot{Identity: identity, Version: foregroundFactVersion(identity, summary), CapturedAt: time.Now(), Summary: summary}, nil
}

// ValidateSnapshot validates the physical-origin contract without exposing
// identity internals. maxAge is enforced when positive; callers choose the
// policy budget rather than the adapter inventing one.
func ValidateSnapshot(snapshot Snapshot, maxAge time.Duration) error {
	if snapshot.Identity.window == 0 || snapshot.Identity.process == 0 || snapshot.Identity.creationTime == 0 ||
		snapshot.CapturedAt.IsZero() || time.Until(snapshot.CapturedAt) > 0 || !validSummary(snapshot.Summary) ||
		snapshot.Version != foregroundFactVersion(snapshot.Identity, snapshot.Summary) {
		return ErrInvalidSnapshot
	}
	if maxAge > 0 && time.Since(snapshot.CapturedAt) > maxAge {
		return ErrInvalidSnapshot
	}
	return nil
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
	if ValidateSnapshot(snapshot, 0) != nil {
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
	value = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(value), "\x00"))
	if !validObservationText(value) || strings.ContainsAny(value, `/\`) || hasDrivePrefix(value) {
		return ""
	}
	return value
}

func validSummary(summary Summary) bool {
	return summary.ProviderVersion == ProviderVersion && validObservationText(summary.Executable) &&
		!strings.ContainsAny(summary.Executable, `/\:`) && normalizeExecutableBase(summary.Executable) == summary.Executable &&
		summary.WindowClass != "" && normalizeWindowClass(summary.WindowClass) == summary.WindowClass
}

func hasDrivePrefix(value string) bool {
	return len(value) >= 2 && value[1] == ':' && (value[0] >= 'A' && value[0] <= 'Z' || value[0] >= 'a' && value[0] <= 'z')
}

func validObservationText(value string) bool {
	if value == "" || !utf8.ValidString(value) || strings.TrimSpace(value) != value || utf8.RuneCountInString(value) > maxWindowClassRunes {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return false
		}
	}
	return true
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
