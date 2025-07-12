// Package change provides entity state management for the GoEF ORM.
package change

import (
	"fmt"
	"time"
)

// EntityStateManager manages the state of entities in the change tracker
type EntityStateManager struct {
	states     map[string]EntityState
	timestamps map[string]time.Time
}

// NewEntityStateManager creates a new entity state manager
func NewEntityStateManager() *EntityStateManager {
	return &EntityStateManager{
		states:     make(map[string]EntityState),
		timestamps: make(map[string]time.Time),
	}
}

// SetState sets the state of an entity
func (esm *EntityStateManager) SetState(entityKey string, state EntityState) {
	esm.states[entityKey] = state
	esm.timestamps[entityKey] = time.Now()
}

// GetState gets the state of an entity
func (esm *EntityStateManager) GetState(entityKey string) (EntityState, bool) {
	state, exists := esm.states[entityKey]
	return state, exists
}

// GetStateTimestamp gets when the entity state was last changed
func (esm *EntityStateManager) GetStateTimestamp(entityKey string) (time.Time, bool) {
	timestamp, exists := esm.timestamps[entityKey]
	return timestamp, exists
}

// RemoveState removes the state tracking for an entity
func (esm *EntityStateManager) RemoveState(entityKey string) {
	delete(esm.states, entityKey)
	delete(esm.timestamps, entityKey)
}

// GetAllStates returns all tracked entity states
func (esm *EntityStateManager) GetAllStates() map[string]EntityState {
	result := make(map[string]EntityState)
	for key, state := range esm.states {
		result[key] = state
	}
	return result
}

// Clear removes all state tracking
func (esm *EntityStateManager) Clear() {
	esm.states = make(map[string]EntityState)
	esm.timestamps = make(map[string]time.Time)
}

// TransitionState transitions an entity from one state to another
func (esm *EntityStateManager) TransitionState(entityKey string, fromState, toState EntityState) error {
	currentState, exists := esm.GetState(entityKey)
	if !exists {
		return fmt.Errorf("entity %s is not being tracked", entityKey)
	}

	if currentState != fromState {
		return fmt.Errorf("entity %s is in state %s, expected %s", entityKey, currentState, fromState)
	}

	if !esm.isValidTransition(fromState, toState) {
		return fmt.Errorf("invalid state transition from %s to %s", fromState, toState)
	}

	esm.SetState(entityKey, toState)
	return nil
}

// isValidTransition checks if a state transition is valid
func (esm *EntityStateManager) isValidTransition(from, to EntityState) bool {
	validTransitions := map[EntityState][]EntityState{
		StateUnchanged: {StateModified, StateDeleted},
		StateAdded:     {StateUnchanged, StateDeleted},
		StateModified:  {StateUnchanged, StateDeleted},
		StateDeleted:   {}, // No transitions from deleted
	}

	allowedStates, exists := validTransitions[from]
	if !exists {
		return false
	}

	for _, allowedState := range allowedStates {
		if allowedState == to {
			return true
		}
	}

	return false
}

// GetEntitiesByState returns all entities in a specific state
func (esm *EntityStateManager) GetEntitiesByState(state EntityState) []string {
	var entities []string
	for entityKey, entityState := range esm.states {
		if entityState == state {
			entities = append(entities, entityKey)
		}
	}
	return entities
}

// GetStateStats returns statistics about entity states
func (esm *EntityStateManager) GetStateStats() map[EntityState]int {
	stats := make(map[EntityState]int)
	for _, state := range esm.states {
		stats[state]++
	}
	return stats
}
