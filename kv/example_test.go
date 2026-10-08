package kv_test

import (
	"fmt"

	"github.com/RockInMars/openbao-sdk-go/kv"
)

func ExampleDocument() {
	document, err := kv.NewDocument(map[string]string{"region": "example"})
	if err != nil {
		panic(err)
	}
	defer document.Zero()
	var decoded map[string]string
	if err := document.Decode(&decoded); err != nil {
		panic(err)
	}
	// Decoded values belong to the caller; Zero does not erase decoded strings.
	fmt.Println(len(decoded), document)
	// Output: 1 [REDACTED]
}
