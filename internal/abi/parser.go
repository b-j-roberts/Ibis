package abi

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ABI represents a parsed Cairo contract ABI with resolved types and events.
type ABI struct {
	Types  map[string]*TypeDef // Full name -> resolved type definition
	Events []*EventDef         // All emittable events (struct-kind events)
}

// ParseFile parses a Cairo ABI from a file path.
// Supports both .contract_class.json files (with embedded ABI string)
// and raw ABI JSON array files.
func ParseFile(path string) (*ABI, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading ABI file: %w", err)
	}
	return Parse(data)
}

// Parse parses Cairo ABI JSON data. Handles three formats:
//   - Contract class JSON with string ABI (compiler <2.7.0): {"abi": "[{...}]", ...}
//   - Contract class JSON with array ABI (compiler >=2.7.0): {"abi": [{...}], ...}
//   - Raw ABI JSON array: [{...}, {...}, ...]
func Parse(data []byte) (*ABI, error) {
	var rawEntries []RawABIEntry

	// Try contract_class.json format first (has "abi" field).
	var classJSON ContractClassJSON
	if err := json.Unmarshal(data, &classJSON); err == nil && len(classJSON.ABI) > 0 {
		abiData := classJSON.ABI

		// If the ABI field is a JSON string, unwrap it first.
		var abiStr string
		if json.Unmarshal(abiData, &abiStr) == nil {
			abiData = []byte(abiStr)
		}

		// Parse the ABI entries (now always a JSON array).
		if err := json.Unmarshal(abiData, &rawEntries); err != nil {
			return nil, fmt.Errorf("parsing ABI from contract class: %w", err)
		}
	} else {
		// Try raw ABI JSON array.
		if err := json.Unmarshal(data, &rawEntries); err != nil {
			return nil, fmt.Errorf("parsing ABI JSON: %w", err)
		}
	}

	return buildABI(rawEntries)
}

// buildABI resolves raw ABI entries into typed definitions.
func buildABI(entries []RawABIEntry) (*ABI, error) {
	abi := &ABI{
		Types: make(map[string]*TypeDef),
	}

	// Pass 1: Register all struct and enum type skeletons.
	for _, entry := range entries {
		switch entry.Type {
		case "struct":
			abi.Types[entry.Name] = &TypeDef{
				Kind: CairoStruct,
				Name: entry.Name,
			}
		case "enum":
			abi.Types[entry.Name] = &TypeDef{
				Kind: CairoEnum,
				Name: entry.Name,
			}
		}
	}

	// Pass 2: Resolve members and variants.
	for _, entry := range entries {
		switch entry.Type {
		case "struct":
			td := abi.Types[entry.Name]
			for _, m := range entry.Members {
				resolved := abi.resolveType(m.Type)
				td.Members = append(td.Members, FieldDef{
					Name: m.Name,
					Type: resolved,
				})
			}
		case "enum":
			td := abi.Types[entry.Name]
			for _, v := range entry.Variants {
				resolved := abi.resolveType(v.Type)
				td.Variants = append(td.Variants, FieldDef{
					Name: v.Name,
					Type: resolved,
				})
			}
		}
	}

	// Pass 3: Extract event definitions.
	for _, entry := range entries {
		if entry.Type != "event" || entry.Kind != "struct" {
			continue
		}
		ev, err := abi.buildEventDef(entry)
		if err != nil {
			return nil, fmt.Errorf("building event %s: %w", entry.Name, err)
		}
		abi.Events = append(abi.Events, ev)
	}

	return abi, nil
}

// buildEventDef creates an EventDef from a struct-kind event entry.
func (a *ABI) buildEventDef(entry RawABIEntry) (*EventDef, error) {
	ev := &EventDef{
		Name:     shortName(entry.Name),
		FullName: entry.Name,
	}
	ev.Selector = ComputeSelector(ev.Name)

	for _, m := range entry.Members {
		resolved := a.resolveType(m.Type)
		field := FieldDef{Name: m.Name, Type: resolved}
		switch m.Kind {
		case "key":
			ev.KeyMembers = append(ev.KeyMembers, field)
		case "data":
			ev.DataMembers = append(ev.DataMembers, field)
		default:
			// Default to data if kind not specified.
			ev.DataMembers = append(ev.DataMembers, field)
		}
	}

	return ev, nil
}

// resolveType resolves a Cairo type string into a TypeDef.
func (a *ABI) resolveType(typeName string) *TypeDef {
	// Strip snapshot prefix.
	typeName = strings.TrimPrefix(typeName, "@")

	// Check primitives and built-in types first.
	// This ensures well-known types like u256 and bool are decoded as
	// primitives even when they appear as struct/enum in the ABI.
	if isKnownPrimitive(typeName) {
		return resolvePrimitive(typeName)
	}

	// Check for registered struct/enum types.
	if td, ok := a.Types[typeName]; ok {
		return td
	}

	// Unknown type -- treat as felt252.
	return resolvePrimitive(typeName)
}

// knownPrimitives is the set of Cairo types that should always be decoded
// as primitives, even when they appear as struct/enum in the ABI.
var knownPrimitives = map[string]bool{
	"core::felt252":                                        true,
	"core::integer::u8":                                    true,
	"core::integer::u16":                                   true,
	"core::integer::u32":                                   true,
	"core::integer::u64":                                   true,
	"core::integer::u128":                                  true,
	"core::integer::u256":                                  true,
	"core::integer::i8":                                    true,
	"core::integer::i16":                                   true,
	"core::integer::i32":                                   true,
	"core::integer::i64":                                   true,
	"core::integer::i128":                                  true,
	"core::bool":                                           true,
	"core::starknet::contract_address::ContractAddress":    true,
	"core::starknet::class_hash::ClassHash":                true,
	"core::byte_array::ByteArray":                          true,
	"()":                                                   true,
	"core::zeroable::NonZero::<()>":                        true,
}

// isKnownPrimitive returns true if the type name is a well-known primitive
// or generic type (Array/Span) that should bypass the ABI type registry.
func isKnownPrimitive(name string) bool {
	if knownPrimitives[name] {
		return true
	}
	if strings.HasPrefix(name, "core::array::Array::<") || strings.HasPrefix(name, "core::array::Span::<") {
		return true
	}
	return false
}

// resolvePrimitive resolves a Cairo type name to a primitive TypeDef.
func resolvePrimitive(name string) *TypeDef {
	switch name {
	case "core::felt252":
		return &TypeDef{Kind: CairoFelt252, Name: name}
	case "core::integer::u8":
		return &TypeDef{Kind: CairoU8, Name: name}
	case "core::integer::u16":
		return &TypeDef{Kind: CairoU16, Name: name}
	case "core::integer::u32":
		return &TypeDef{Kind: CairoU32, Name: name}
	case "core::integer::u64":
		return &TypeDef{Kind: CairoU64, Name: name}
	case "core::integer::u128":
		return &TypeDef{Kind: CairoU128, Name: name}
	case "core::integer::u256":
		return &TypeDef{Kind: CairoU256, Name: name}
	case "core::integer::i8":
		return &TypeDef{Kind: CairoI8, Name: name}
	case "core::integer::i16":
		return &TypeDef{Kind: CairoI16, Name: name}
	case "core::integer::i32":
		return &TypeDef{Kind: CairoI32, Name: name}
	case "core::integer::i64":
		return &TypeDef{Kind: CairoI64, Name: name}
	case "core::integer::i128":
		return &TypeDef{Kind: CairoI128, Name: name}
	case "core::bool":
		return &TypeDef{Kind: CairoBool, Name: name}
	case "core::starknet::contract_address::ContractAddress":
		return &TypeDef{Kind: CairoContractAddress, Name: name}
	case "core::starknet::class_hash::ClassHash":
		return &TypeDef{Kind: CairoClassHash, Name: name}
	case "core::byte_array::ByteArray":
		return &TypeDef{Kind: CairoByteArray, Name: name}
	case "()", "core::zeroable::NonZero::<()>":
		return &TypeDef{Kind: CairoUnit, Name: name}
	}

	// Handle generic types: Array<T>, Span<T>.
	if inner, ok := extractGenericInner(name, "core::array::Array::<"); ok {
		return &TypeDef{Kind: CairoArray, Name: name, Inner: resolvePrimitive(inner)}
	}
	if inner, ok := extractGenericInner(name, "core::array::Span::<"); ok {
		return &TypeDef{Kind: CairoSpan, Name: name, Inner: resolvePrimitive(inner)}
	}

	// Unknown type -- treat as felt252 (safest default for unknown felts).
	return &TypeDef{Kind: CairoFelt252, Name: name}
}

// extractGenericInner extracts the inner type from a generic type string.
// e.g., "core::array::Array::<core::integer::u64>" -> "core::integer::u64", true
func extractGenericInner(name, prefix string) (string, bool) {
	if !strings.HasPrefix(name, prefix) {
		return "", false
	}
	inner := strings.TrimPrefix(name, prefix)
	if !strings.HasSuffix(inner, ">") {
		return "", false
	}
	inner = strings.TrimSuffix(inner, ">")
	return inner, true
}

// shortName extracts the last segment from a full qualified name.
// e.g., "openzeppelin::token::erc20::Transfer" -> "Transfer"
func shortName(fullName string) string {
	parts := strings.Split(fullName, "::")
	return parts[len(parts)-1]
}
