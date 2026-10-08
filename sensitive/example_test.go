package sensitive_test

import (
	"fmt"

	"github.com/RockInMars/openbao-sdk-go/sensitive"
)

func ExampleBytes() {
	input := []byte("sample")
	value := sensitive.NewBytes(input)
	clear(input)
	alias := value // A shared erasure handle, not an independent clone.
	raw := value.RevealCopy()
	independent := sensitive.NewBytes(raw)
	clear(raw)
	defer independent.Zero()
	fmt.Println(value) // Formatting never reveals plaintext.
	value.Zero()
	fmt.Println(alias.Len(), independent.Len())
	// Output:
	// [REDACTED]
	// 0 6
}
