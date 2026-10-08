package transitutil

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func BenchmarkTransitEncoding(b *testing.B) {
	for _, size := range []struct {
		name  string
		bytes int
	}{{"payload_32", 32}, {"payload_4096", 4096}} {
		b.Run(size.name, func(b *testing.B) {
			raw := bytes.Repeat([]byte{0x5a}, size.bytes)
			b.SetBytes(int64(len(raw)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				wire := "vault:v1:" + base64.StdEncoding.EncodeToString(raw)
				version, decoded, err := Unwrap(wire, len(wire))
				if err != nil || version != 1 || !bytes.Equal(decoded, raw) {
					b.Fatal("encoding round trip failed")
				}
				clear(decoded)
			}
		})
	}
}
