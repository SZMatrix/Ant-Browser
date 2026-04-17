package extension

import (
    "bytes"
    "encoding/binary"
    "testing"
)

// synth builds the smallest valid CRX3 byte stream wrapping the given zip bytes.
// Header layout: magic("Cr24")(4) | version=3(4) | headerLen(4) | header(headerLen) | zip
func synthCRX3(headerBody, zipBytes []byte) []byte {
    var buf bytes.Buffer
    buf.WriteString("Cr24")
    binary.Write(&buf, binary.LittleEndian, uint32(3))
    binary.Write(&buf, binary.LittleEndian, uint32(len(headerBody)))
    buf.Write(headerBody)
    buf.Write(zipBytes)
    return buf.Bytes()
}

func TestDetectKind(t *testing.T) {
    zip := []byte("PK\x03\x04zipbody")
    crx := synthCRX3([]byte("fake-header-body"), zip)

    if DetectKind(crx) != "crx" { t.Fatal("want crx") }
    if DetectKind(zip) != "zip" { t.Fatal("want zip") }
    if DetectKind([]byte("")) != "zip" { t.Fatal("empty defaults to zip") }
}

func TestStripCRXHeader(t *testing.T) {
    zip := []byte("PK\x03\x04zipbody")
    crx := synthCRX3([]byte("fake-header-body"), zip)

    out, chromeID, err := StripCRXHeader(crx)
    if err != nil { t.Fatal(err) }
    if !bytes.Equal(out, zip) {
        t.Fatalf("stripped zip mismatch: got=%q want=%q", out, zip)
    }
    // fake-header-body is not a real proto; chromeID should be "" without erroring.
    _ = chromeID
}

func TestStripCRXHeaderRejectsBadMagic(t *testing.T) {
    if _, _, err := StripCRXHeader([]byte("not-a-crx")); err == nil {
        t.Fatal("want error for bad magic")
    }
}

// Chrome IDs use only characters a..p (the mapped base-16 alphabet). Regression
// test against the previous hex→a-p mapping that emitted invalid bytes for a-f.
func TestDeriveChromeIDAlphabet(t *testing.T) {
    id := deriveChromeID([]byte("example public key bytes"))
    if len(id) != 32 {
        t.Fatalf("want 32 chars, got %d (%q)", len(id), id)
    }
    for i := 0; i < len(id); i++ {
        c := id[i]
        if c < 'a' || c > 'p' {
            t.Fatalf("id[%d]=%q is outside a..p (got id=%q)", i, c, id)
        }
    }
    if deriveChromeID(nil) != "" {
        t.Fatal("empty key must return empty id")
    }
}
