package abi

import (
	"fmt"
	"strings"

	"github.com/NethermindEth/juno/core/felt"
)

// EncodeFunctionCalldata converts hex string calldata arguments into []*felt.Felt.
// Each argument must be a 0x-prefixed hex string representing a felt value.
func EncodeFunctionCalldata(args []string) ([]*felt.Felt, error) {
	result := make([]*felt.Felt, 0, len(args))
	for i, arg := range args {
		arg = strings.TrimSpace(arg)
		if !strings.HasPrefix(arg, "0x") && !strings.HasPrefix(arg, "0X") {
			return nil, fmt.Errorf("calldata[%d]: must start with 0x, got %q", i, arg)
		}
		f, err := new(felt.Felt).SetString(arg)
		if err != nil {
			return nil, fmt.Errorf("calldata[%d]: invalid hex felt %q: %w", i, arg, err)
		}
		result = append(result, f)
	}
	return result, nil
}
