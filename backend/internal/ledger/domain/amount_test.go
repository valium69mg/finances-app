package domain_test

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
)

func TestCheckAmount(t *testing.T) {
	for _, tc := range []struct {
		in string
		ok bool
	}{
		{"0", true},
		{"0.01", true},
		{"-5", true}, // the sign is the caller's rule
		{"999999999999.99", true},
		{"-999999999999.99", true},
		{"1e11", true},
		{"10000000000000", false}, // 1e13
		{"1000000000000", false},  // MaxAmount itself is exclusive
		{"-1000000000000", false},
		{"1e12", false},
		{"1e2000000000", false},
		{"-1e2000000000", false},
		{"1e999999999", false},
		{"1e-2000000000", false},
		{"0.00000000000000000000000000000000001", false}, // 35 decimals
	} {
		v, err := decimal.NewFromString(tc.in)
		if err != nil {
			t.Fatalf("%s: %v", tc.in, err)
		}
		err = ledger.CheckAmount(v)
		if tc.ok && err != nil {
			t.Errorf("CheckAmount(%s) = %v, want nil", tc.in, err)
		}
		if !tc.ok && !errors.Is(err, ledger.ErrAmountOutOfRange) {
			t.Errorf("CheckAmount(%s) = %v, want ErrAmountOutOfRange", tc.in, err)
		}
	}
}
