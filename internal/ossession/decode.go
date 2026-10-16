package ossession

import (
	"encoding/binary"
	"fmt"
)

const (
	wtsInfoExLevel            = 1
	wtsConnectStateActive     = uint32(0)
	wtsSessionStateLock       = int32(0)
	wtsSessionStateUnlock     = int32(1)
	wtsSessionStateUnknown    = int32(-1)
	wtsInfoExLevel1PrefixSize = 12
)

// decodeWTSInfoEx lê somente o prefixo necessário do WTSINFOEXW. O offset é
// 8 no ABI Windows suportado, pois a união contém LARGE_INTEGER com
// alinhamento /Zp8. O restante do buffer continua opaco e é liberado pelo
// chamador que o recebeu de WTSQuerySessionInformationW.
func decodeWTSInfoEx(buffer []byte, dataOffset int, expectedSession uint32) (State, error) {
	if dataOffset != 8 || len(buffer) < dataOffset || len(buffer)-dataOffset < wtsInfoExLevel1PrefixSize {
		return unknownState(), fmt.Errorf("ossession: invalid WTSINFOEX buffer or alignment")
	}
	if len(buffer) < 4 {
		return unknownState(), fmt.Errorf("ossession: WTSINFOEX header is missing")
	}
	if binary.LittleEndian.Uint32(buffer[:4]) != wtsInfoExLevel {
		return unknownState(), fmt.Errorf("ossession: unsupported WTSINFOEX level")
	}

	prefix := buffer[dataOffset:]
	sessionID := binary.LittleEndian.Uint32(prefix[0:4])
	if sessionID != expectedSession {
		return unknownState(), fmt.Errorf("ossession: WTSINFOEX session mismatch: got %d, want %d", sessionID, expectedSession)
	}
	if binary.LittleEndian.Uint32(prefix[4:8]) != wtsConnectStateActive {
		return unknownState(), fmt.Errorf("ossession: WTSINFOEX session is not active")
	}
	flags := int32(binary.LittleEndian.Uint32(prefix[8:12]))
	switch flags {
	case wtsSessionStateLock:
		return State{Known: true, Locked: true}, nil
	case wtsSessionStateUnlock:
		return State{Known: true, Locked: false}, nil
	case wtsSessionStateUnknown:
		return unknownState(), fmt.Errorf("ossession: WTSINFOEX session state is unknown")
	default:
		return unknownState(), fmt.Errorf("ossession: invalid WTSINFOEX session flags: %d", flags)
	}
}
