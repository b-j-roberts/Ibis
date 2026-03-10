//go:build deps

// Package internal pins module dependencies that are not yet imported in code.
// This file is excluded from normal builds via the "deps" build tag.
package internal

import (
	_ "github.com/NethermindEth/starknet.go/rpc"
	_ "github.com/georgysavva/scany/v2/pgxscan"
	_ "github.com/jackc/pgx/v5"
)
