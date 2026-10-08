package kv

import (
	"strings"
	"testing"
)

func BenchmarkDocument(b *testing.B) {
	for _, size := range []struct {
		name  string
		bytes int
	}{{"small_64", 64}, {"large_65536", 65536}} {
		b.Run(size.name, func(b *testing.B) {
			raw := []byte(`{"value":"` + strings.Repeat("x", size.bytes) + `","version":1}`)
			b.SetBytes(int64(len(raw)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				doc, err := ParseDocument(raw)
				if err != nil {
					b.Fatal(err)
				}
				var dst struct {
					Value   string
					Version int
				}
				err = doc.Decode(&dst)
				doc.Zero()
				if err != nil || len(dst.Value) != size.bytes || dst.Version != 1 {
					b.Fatal("document round trip failed")
				}
			}
		})
	}
}
