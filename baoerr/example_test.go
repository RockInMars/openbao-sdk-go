package baoerr_test

import (
	"fmt"

	"github.com/RockInMars/openbao-sdk-go/baoerr"
)

func ExampleHasUnknownOutcome() {
	// An illustrative lost write response, not an executed server operation.
	err := fmt.Errorf("save: %w", &baoerr.Error{
		Code: baoerr.CodeDeadlineExceeded, Effect: baoerr.EffectUnknown,
	})
	if baoerr.HasUnknownOutcome(err) {
		fmt.Println("reconcile server state before retrying")
	}
	fmt.Println(baoerr.IsCode(err, baoerr.CodeDeadlineExceeded))
	// Output:
	// reconcile server state before retrying
	// true
}
