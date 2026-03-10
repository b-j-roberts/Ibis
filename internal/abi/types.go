package abi

import (
	"github.com/NethermindEth/juno/core/felt"
)

// CairoType classifies Cairo type kinds for decoding.
type CairoType int

const (
	CairoFelt252 CairoType = iota
	CairoU8
	CairoU16
	CairoU32
	CairoU64
	CairoU128
	CairoU256
	CairoI8
	CairoI16
	CairoI32
	CairoI64
	CairoI128
	CairoBool
	CairoContractAddress
	CairoClassHash
	CairoByteArray
	CairoArray
	CairoSpan
	CairoStruct
	CairoEnum
	CairoUnit // ()
)

// TypeDef represents a resolved Cairo type definition.
type TypeDef struct {
	Kind     CairoType
	Name     string     // Full qualified name (e.g., "core::integer::u256")
	Inner    *TypeDef   // For Array/Span: element type
	Members  []FieldDef // For Struct: ordered fields
	Variants []FieldDef // For Enum: variants
}

// FeltSize returns how many felts this type consumes when serialized.
func (t *TypeDef) FeltSize() int {
	switch t.Kind {
	case CairoUnit:
		return 0
	case CairoFelt252, CairoU8, CairoU16, CairoU32, CairoU64, CairoU128,
		CairoI8, CairoI16, CairoI32, CairoI64, CairoI128,
		CairoBool, CairoContractAddress, CairoClassHash:
		return 1
	case CairoU256:
		return 2 // low + high u128
	case CairoStruct:
		size := 0
		for _, m := range t.Members {
			size += m.Type.FeltSize()
		}
		return size
	case CairoEnum:
		return -1 // Variable size, depends on variant
	case CairoArray, CairoSpan:
		return -1 // Variable size, length-prefixed
	case CairoByteArray:
		return -1 // Variable size
	default:
		return 1
	}
}

// FieldDef represents a named field within a struct, enum variant, or event member.
type FieldDef struct {
	Name string
	Type *TypeDef
}

// EventDef represents a parsed and resolved event definition.
type EventDef struct {
	Name        string      // Short name (e.g., "Transfer")
	FullName    string      // Full qualified name (e.g., "openzeppelin::...::Transfer")
	Selector    *felt.Felt  // sn_keccak(Name)
	KeyMembers  []FieldDef  // Members encoded in keys[] (after keys[0] selector)
	DataMembers []FieldDef  // Members encoded in data[]
}

// --- Raw JSON types for parsing Cairo ABI ---

// RawABIEntry represents a single entry in a Cairo ABI JSON array.
type RawABIEntry struct {
	Type            string        `json:"type"`             // "struct", "enum", "event", "function", "interface", "impl"
	Name            string        `json:"name"`             // Full qualified name
	Members         []RawMember   `json:"members,omitempty"`
	Variants        []RawVariant  `json:"variants,omitempty"`
	Kind            string        `json:"kind,omitempty"`   // For events: "struct" or "enum"
	Items           []RawABIEntry `json:"items,omitempty"`  // For interface
	Inputs          []RawParam    `json:"inputs,omitempty"`
	Outputs         []RawParam    `json:"outputs,omitempty"`
	StateMutability string        `json:"state_mutability,omitempty"`
}

// RawMember represents a struct member or event member in the ABI JSON.
type RawMember struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Kind string `json:"kind,omitempty"` // For event members: "key" or "data"
}

// RawVariant represents an enum variant in the ABI JSON.
type RawVariant struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Kind string `json:"kind,omitempty"` // For event variants: "nested" or "flat"
}

// RawParam represents a function input/output parameter.
type RawParam struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// ContractClassJSON represents the top-level structure of a .contract_class.json file.
type ContractClassJSON struct {
	ABI string `json:"abi"` // String-encoded JSON array of ABI entries
}
