package strerr_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/go-cnfg/cnfg/strerr"
)

const errTest = strerr.Error("test error")

func TestError(t *testing.T) {
	if errTest.Error() != "test error" {
		t.Errorf("got %q", errTest.Error())
	}

	wrapped := fmt.Errorf("wrapped: %w", errTest)
	if !errors.Is(wrapped, errTest) {
		t.Error("wrapped error should match")
	}
}
