package extension

import (
    "bytes"
    "crypto/sha256"
    "encoding/binary"
    "errors"
    "fmt"
)

// DetectKind inspects magic bytes; returns "crx" if CRX, otherwise "zip".
func DetectKind(raw []byte) string {
    if len(raw) >= 4 && bytes.Equal(raw[:4], []byte("Cr24")) { return "crx" }
    return "zip"
}

// StripCRXHeader removes the CRX2 or CRX3 header from raw and returns the
// embedded ZIP plus the derived 32-char Chrome ID when the header contains a
// usable public key. For CRX3 the first declared sha256_with_rsa key is used;
// if none is present the chrome_id is returned empty (installer will not set
// DuplicateOf in that case, which is the intended behavior).
func StripCRXHeader(raw []byte) ([]byte, string, error) {
    if len(raw) < 12 || !bytes.Equal(raw[:4], []byte("Cr24")) {
        return nil, "", errors.New("invalid CRX magic")
    }
    version := binary.LittleEndian.Uint32(raw[4:8])
    switch version {
    case 2:
        if len(raw) < 16 {
            return nil, "", errors.New("CRX2 header truncated")
        }
        pubKeyLen := binary.LittleEndian.Uint32(raw[8:12])
        sigLen := binary.LittleEndian.Uint32(raw[12:16])
        start := 16 + int(pubKeyLen) + int(sigLen)
        if start > len(raw) { return nil, "", errors.New("CRX2 truncated") }
        pubKey := raw[16 : 16+int(pubKeyLen)]
        return raw[start:], deriveChromeID(pubKey), nil
    case 3:
        headerLen := binary.LittleEndian.Uint32(raw[8:12])
        start := 12 + int(headerLen)
        if start > len(raw) { return nil, "", errors.New("CRX3 truncated") }
        id := firstRSAPubKeyChromeID(raw[12:start])
        return raw[start:], id, nil
    default:
        return nil, "", fmt.Errorf("unsupported CRX version %d", version)
    }
}

// firstRSAPubKeyChromeID is a best-effort scan of the CRX3 header protobuf.
// It walks top-level fields looking for sha256_with_rsa entries (field 2,
// wire type 2 = LEN) inside AsymmetricKeyProof, extracting the first
// public_key (field 1, LEN). On any parse error we return "" (caller treats
// absent chrome_id as "no duplicate match").
func firstRSAPubKeyChromeID(header []byte) string {
    b := header
    for len(b) > 0 {
        tag, n := readVarint(b)
        if n <= 0 { return "" }
        b = b[n:]
        fieldNum := tag >> 3
        wire := tag & 0x7
        if wire != 2 {
            return ""
        }
        length, n2 := readVarint(b)
        if n2 <= 0 || int(length) > len(b[n2:]) { return "" }
        payload := b[n2 : n2+int(length)]
        b = b[n2+int(length):]
        if fieldNum == 2 {
            inner := payload
            for len(inner) > 0 {
                itag, in := readVarint(inner)
                if in <= 0 { break }
                inner = inner[in:]
                ifield := itag >> 3
                iwire := itag & 0x7
                if iwire != 2 { break }
                ilen, in2 := readVarint(inner)
                if in2 <= 0 || int(ilen) > len(inner[in2:]) { break }
                val := inner[in2 : in2+int(ilen)]
                inner = inner[in2+int(ilen):]
                if ifield == 1 {
                    return deriveChromeID(val)
                }
            }
        }
    }
    return ""
}

// deriveChromeID returns the 32-char Chrome extension ID from the raw public key bytes.
// Algorithm: sha256(pubKey) → take first 16 bytes → map each nibble 0..15 to 'a'..'p'.
func deriveChromeID(pubKey []byte) string {
    if len(pubKey) == 0 { return "" }
    sum := sha256.Sum256(pubKey)
    const alphabet = "abcdefghijklmnop"
    b := make([]byte, 32)
    for i := 0; i < 16; i++ {
        b[2*i] = alphabet[sum[i]>>4]
        b[2*i+1] = alphabet[sum[i]&0x0f]
    }
    return string(b)
}

func readVarint(b []byte) (uint64, int) {
    var x uint64
    for i, c := range b {
        if i >= 10 { return 0, 0 }
        x |= uint64(c&0x7F) << (7 * uint(i))
        if c&0x80 == 0 { return x, i + 1 }
    }
    return 0, 0
}
