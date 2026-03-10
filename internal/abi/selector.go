package abi

import (
	"github.com/NethermindEth/juno/core/felt"
	"github.com/NethermindEth/starknet.go/utils"
)

// EventRegistry maps event selectors to their definitions for fast lookup.
type EventRegistry struct {
	bySelector map[felt.Felt]*EventDef
	byName     map[string]*EventDef
}

// NewEventRegistry builds a registry from parsed ABI events.
func NewEventRegistry(abi *ABI) *EventRegistry {
	reg := &EventRegistry{
		bySelector: make(map[felt.Felt]*EventDef, len(abi.Events)),
		byName:     make(map[string]*EventDef, len(abi.Events)),
	}
	for _, ev := range abi.Events {
		reg.bySelector[*ev.Selector] = ev
		reg.byName[ev.Name] = ev
	}
	return reg
}

// MatchSelector finds the event definition matching a selector felt (typically keys[0]).
// Returns nil if no match is found.
func (r *EventRegistry) MatchSelector(selector *felt.Felt) *EventDef {
	if selector == nil {
		return nil
	}
	ev, ok := r.bySelector[*selector]
	if !ok {
		return nil
	}
	return ev
}

// MatchName finds the event definition by short name.
// Returns nil if no match is found.
func (r *EventRegistry) MatchName(name string) *EventDef {
	ev, ok := r.byName[name]
	if !ok {
		return nil
	}
	return ev
}

// Events returns all registered event definitions.
func (r *EventRegistry) Events() []*EventDef {
	events := make([]*EventDef, 0, len(r.bySelector))
	for _, ev := range r.bySelector {
		events = append(events, ev)
	}
	return events
}

// ComputeSelector computes the starknet_keccak selector for an event name.
func ComputeSelector(name string) *felt.Felt {
	return utils.GetSelectorFromNameFelt(name)
}
