package pluginpackage

import (
	"slices"
	"testing"

	"golang.org/x/sys/windows"
)

func TestPackageOriginVolumeNameFallback(t *testing.T) {
	const guid = `\\?\Volume{01234567-89ab-cdef-0123-456789abcdef}\package`
	for _, tc := range []struct {
		name            string
		dosErr, guidErr error
		wantFlags       []uint32
		wantErr         error
	}{
		{"volume without DOS path", windows.ERROR_PATH_NOT_FOUND, nil, []uint32{0, volumeNameGUID}, nil},
		{"GUID lookup fails", windows.ERROR_PATH_NOT_FOUND, windows.ERROR_PATH_NOT_FOUND, []uint32{0, volumeNameGUID}, windows.ERROR_PATH_NOT_FOUND},
		{"access denied", windows.ERROR_ACCESS_DENIED, nil, []uint32{0}, windows.ERROR_ACCESS_DENIED},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var flagsUsed []uint32
			path, err := finalPackagePath(0, func(_ windows.Handle, flags uint32) (string, error) {
				flagsUsed = append(flagsUsed, flags)
				if len(flagsUsed) > len(tc.wantFlags) {
					t.Fatal("unexpected retry")
				}
				if flags == 0 {
					return "", tc.dosErr
				}
				if tc.guidErr != nil {
					return "", tc.guidErr
				}
				return guid, nil
			})
			if err != tc.wantErr || !slices.Equal(flagsUsed, tc.wantFlags) || (err == nil && path != guid) {
				t.Fatalf("path %q, error %v, flags %v", path, err, flagsUsed)
			}
		})
	}
}
