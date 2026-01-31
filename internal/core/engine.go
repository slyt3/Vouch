package core

import (
	"sync"

	"github.com/slyt3/Logryph/internal/ledger"
	"github.com/slyt3/Logryph/internal/observer"
)

// Engine is the central state manager for Logryph
type Engine struct {
	Worker          *ledger.Worker
	ActiveTasks     *sync.Map // task_id -> state
	Observer        *observer.ObserverEngine
	LastEventByTask *sync.Map // task_id -> last_event_id
}

// NewEngine creates a new core state engine
func NewEngine(worker *ledger.Worker, obs *observer.ObserverEngine) *Engine {
	return &Engine{
		Worker:          worker,
		Observer:        obs,
		ActiveTasks:     &sync.Map{},
		LastEventByTask: &sync.Map{},
	}
}
