package core

import (
	"sync"

	"github.com/yourname/vouch/internal/ledger"
	"github.com/yourname/vouch/internal/proxy"
)

// Engine is the central state manager for Vouch
type Engine struct {
	Worker          *ledger.Worker
	ActiveTasks     *sync.Map // task_id -> state
	Policy          *proxy.PolicyConfig
	StallSignals    *sync.Map // Maps event ID to approval channel
	LastEventByTask *sync.Map // task_id -> last_event_id
}

// NewEngine creates a new core state engine
func NewEngine(worker *ledger.Worker, policy *proxy.PolicyConfig) *Engine {
	return &Engine{
		Worker:          worker,
		Policy:          policy,
		ActiveTasks:     &sync.Map{},
		StallSignals:    &sync.Map{},
		LastEventByTask: &sync.Map{},
	}
}
