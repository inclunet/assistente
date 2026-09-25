package commandjson

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCanonicalizeRFC8785Example(t *testing.T) {
	raw := []byte(`{
        "numbers": [333333333.33333329, 1E30, 4.50,
                    2e-3, 0.000000000000000000000000001],
        "string": "\u20ac$\u000F\u000aA'\u0042\u0022\u005c\\\"\/",
        "literals": [null, true, false]
    }`)
	want := `{"literals":[null,true,false],"numbers":[333333333.3333333,1e+30,4.5,0.002,1e-27],"string":"€$\u000f\nA'B` + string([]byte{92, 34, 92, 92, 92, 92, 92, 34, 47}) + `"}`
	got, err := Canonicalize(raw)
	if err != nil {
		t.Fatalf("Canonicalize() error = %v", err)
	}
	if string(got) != want {
		t.Fatalf("canonical = %s, want %s", got, want)
	}
}

func TestCanonicalizeAppendixBNumbers(t *testing.T) {
	tests := []struct {
		bits uint64
		want string
	}{
		{0x0000000000000000, "0"},
		{0x8000000000000000, "0"},
		{0x0000000000000001, "5e-324"},
		{0x8000000000000001, "-5e-324"},
		{0x7fefffffffffffff, "1.7976931348623157e+308"},
		{0xffefffffffffffff, "-1.7976931348623157e+308"},
		{0x4340000000000000, "9007199254740992"},
		{0xc340000000000000, "-9007199254740992"},
		{0x4430000000000000, "295147905179352830000"},
		{0x44b52d02c7e14af5, "9.999999999999997e+22"},
		{0x44b52d02c7e14af6, "1e+23"},
		{0x44b52d02c7e14af7, "1.0000000000000001e+23"},
		{0x444b1ae4d6e2ef4e, "999999999999999700000"},
		{0x444b1ae4d6e2ef4f, "999999999999999900000"},
		{0x444b1ae4d6e2ef50, "1e+21"},
		{0x3eb0c6f7a0b5ed8c, "9.999999999999997e-7"},
		{0x3eb0c6f7a0b5ed8d, "0.000001"},
		{0x41b3de4355555553, "333333333.3333332"},
		{0x41b3de4355555554, "333333333.33333325"},
		{0x41b3de4355555555, "333333333.3333333"},
		{0x41b3de4355555556, "333333333.3333334"},
		{0x41b3de4355555557, "333333333.33333343"},
		{0xbecbf647612f3696, "-0.0000033333333333333333"},
		{0x43143ff3c1cb0959, "1424953923781206.2"},
	}
	for _, test := range tests {
		raw := []byte(numberInput(math.Float64frombits(test.bits)))
		got, err := Canonicalize(raw)
		if err != nil {
			t.Errorf("bits %016x: error = %v", test.bits, err)
			continue
		}
		if string(got) != test.want {
			t.Errorf("bits %016x: got %s, want %s", test.bits, got, test.want)
		}
	}
	if _, err := Canonicalize([]byte("1e400")); !errors.Is(err, ErrNumberRange) {
		t.Errorf("overflow: got error %v, want ErrNumberRange", err)
	}
	if _, err := Marshal(math.NaN()); !errors.Is(err, ErrNumberRange) {
		t.Errorf("NaN: got error %v, want ErrNumberRange", err)
	}
}

func TestCanonicalizeSortsUTF16AndRecursively(t *testing.T) {
	raw := []byte(`{"\u20ac":"Euro Sign","\r":"Carriage Return","\ufb33":"Hebrew Letter Dalet With Dagesh","1":"One","\ud83d\ude00":"Emoji: Grinning Face","\u0080":"Control","\u00f6":"Latin Small Letter O With Diaeresis","nested":{"b":1,"a":2}}`)
	want := `{"\r":"Carriage Return","1":"One","nested":{"a":2,"b":1},"` + "\u0080" + `":"Control","ö":"Latin Small Letter O With Diaeresis","€":"Euro Sign","😀":"Emoji: Grinning Face","דּ":"Hebrew Letter Dalet With Dagesh"}`
	got, err := Canonicalize(raw)
	if err != nil {
		t.Fatalf("Canonicalize() error = %v", err)
	}
	if string(got) != want {
		t.Fatalf("canonical = %s, want %s", got, want)
	}
}

func TestCanonicalizeRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want error
	}{
		{"duplicate recursive", []byte(`{"outer":{"a":1,"a":2}}`), ErrDuplicateKey},
		{"duplicate escaped", []byte(`{"a":1,"\u0061":2}`), ErrDuplicateKey},
		{"lone high surrogate", []byte(`"\ud800"`), ErrInvalidUnicode},
		{"lone low surrogate", []byte(`"\udc00"`), ErrInvalidUnicode},
		{"invalid surrogate pair", []byte(`"\ud800\u0041"`), ErrInvalidUnicode},
		{"trailing value", []byte(`{} {}`), ErrInvalidJSON},
		{"trailing byte", []byte(`{}x`), ErrInvalidJSON},
		{"invalid UTF-8", []byte{'"', 0xff, '"'}, ErrInvalidUnicode},
		{"number overflow", []byte(`1e400`), ErrNumberRange},
		{"number underflow", []byte(`1e-400`), ErrNumberRange},
		{"zero exponent", []byte(`0e3`), nil},
		{"negative zero exponent", []byte(`-0E-1`), nil},
		{"zero fraction exponent", []byte(`0.0e999`), nil},
		{"leading zero", []byte(`01`), ErrInvalidJSON},
		{"array trailing comma", []byte(`[1,]`), ErrInvalidJSON},
		{"object missing colon", []byte(`{"a",1}`), ErrInvalidJSON},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Canonicalize(test.raw)
			if test.want == nil && err != nil {
				t.Fatalf("error = %v, want nil", err)
			}
			if test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
	if _, err := Canonicalize(bytes.Repeat([]byte{' '}, maxDocumentSize+1)); !errors.Is(err, ErrDocumentTooLarge) {
		t.Fatalf("large document error = %v", err)
	}
}

func TestCanonicalizeDepthLimit(t *testing.T) {
	for _, test := range []struct {
		containers int
		wantErr    error
	}{
		{containers: maxDepth, wantErr: nil},
		{containers: maxDepth + 1, wantErr: ErrDepthExceeded},
	} {
		raw := []byte(strings.Repeat("[", test.containers) + "0" + strings.Repeat("]", test.containers))
		_, err := Canonicalize(raw)
		if !errors.Is(err, test.wantErr) {
			t.Fatalf("depth %d: error = %v, want %v", test.containers, err, test.wantErr)
		}
	}
}

func TestMarshalUsesJCSAndDoesNotHTMLEscape(t *testing.T) {
	value := map[string]any{"z": "<tag>\u2028", "a": math.Copysign(0, -1), "n": 1e20}
	got, err := Marshal(value)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	want := `{"a":0,"n":100000000000000000000,"z":"<tag> "}`
	if string(got) != want {
		t.Fatalf("Marshal() = %s, want %s", got, want)
	}
	if _, err := Marshal(string([]byte{0xff})); !errors.Is(err, ErrInvalidUnicode) {
		t.Fatalf("invalid string error = %v, want ErrInvalidUnicode", err)
	}
}

func TestMarshalIgnoresPrivateAndExcludedFields(t *testing.T) {
	type node struct {
		Next  *node
		Value string
	}
	type envelope struct {
		At      time.Time `json:"at"`
		Visible string    `json:"visible"`
		Ignored string    `json:"-"`
		private string
		cycle   *node
	}
	cycle := &node{Value: string([]byte{0xff})}
	cycle.Next = cycle

	for _, at := range []time.Time{
		time.Date(2026, time.September, 14, 10, 20, 30, 123456789, time.UTC),
		time.Date(2026, time.September, 14, 7, 20, 30, 123456789, time.FixedZone("local", -3*60*60)),
	} {
		got, err := Marshal(envelope{At: at, Visible: "ok", Ignored: string([]byte{0xff}), private: string([]byte{0xff}), cycle: cycle})
		if err != nil {
			t.Fatalf("Marshal(%v) error = %v", at.Location(), err)
		}
		if !bytes.Contains(got, []byte(`"at":`)) || !bytes.Contains(got, []byte(`"visible":"ok"`)) {
			t.Fatalf("envelope perdeu campos serializáveis: %s", got)
		}
		if bytes.Contains(got, []byte("Ignored")) || bytes.Contains(got, []byte("private")) || bytes.Contains(got, []byte("cycle")) {
			t.Fatalf("Marshal vazou campo ignorado: %s", got)
		}
	}
}

func TestHMACUsesCanonicalPayloadAndUnambiguousDomain(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	got, err := HMAC(key, "arguments", []byte(` { "b": 2, "a": 1 } `))
	if err != nil {
		t.Fatalf("HMAC() error = %v", err)
	}
	want := testHMAC(key, "arguments", []byte(`{"a":1,"b":2}`))
	if got != want {
		t.Fatalf("HMAC() = %s, want %s", got, want)
	}
	otherDomain, err := HMAC(key, "request", []byte(`{"a":1,"b":2}`))
	if err != nil || otherDomain == got {
		t.Fatalf("domains are not separated: arguments=%s request=%s err=%v", got, otherDomain, err)
	}
	if _, err := HMAC(key[:31], "arguments", []byte(`{}`)); !errors.Is(err, ErrInvalidHMAC) {
		t.Fatalf("short key error = %v, want ErrInvalidHMAC", err)
	}
	if _, err := HMAC(key, "", []byte(`{}`)); !errors.Is(err, ErrInvalidHMAC) {
		t.Fatalf("empty domain error = %v, want ErrInvalidHMAC", err)
	}
}

func testHMAC(key []byte, domain string, canonical []byte) string {
	var framed bytes.Buffer
	framed.WriteString("assistente:commandjson:hmac:v1\x00")
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(domain)))
	framed.Write(length[:])
	framed.WriteString(domain)
	binary.BigEndian.PutUint64(length[:], uint64(len(canonical)))
	framed.Write(length[:])
	framed.Write(canonical)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(framed.Bytes())
	return hex.EncodeToString(mac.Sum(nil))
}

func numberInput(value float64) string {
	if math.IsNaN(value) {
		return "NaN"
	}
	if math.IsInf(value, 1) {
		return "1e400"
	}
	return strconv.FormatFloat(value, 'g', -1, 64)
}
