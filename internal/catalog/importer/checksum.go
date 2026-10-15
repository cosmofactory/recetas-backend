package importer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Checksum is the hex SHA-256 of the bundle's canonical JSON: the encoding of
// the parsed Bundle, so the input's key order and whitespace don't matter.
func Checksum(b Bundle) string {
	h := sha256.New()
	data, err := json.Marshal(b)
	if err != nil {
		// Only NaN or infinite quantities fail to encode, and Validate rejects them.
		data = fmt.Appendf(nil, "%#v", b)
	}
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}
