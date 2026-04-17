package extension

import "encoding/base64"

func stdEncodeBase64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }
