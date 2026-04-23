// Package cmd provides the command registry for the CLI application.
package cmd

import (
	"sort"
	"sync"

	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/database"
)

// Command is the interface for all CLI commands.
type Command interface {
	Run(args []string) int
	PrintHelp()
}

// CommandFactory is a function that creates a Command with the given root context.
type CommandFactory func(root *RootCommand) Command

// commandRegistry holds all registered command factories.
var (
	commandRegistry = make(map[string]CommandFactory)
	registryMu      sync.RWMutex
)

// RegisterCommand adds a command factory to the global registry.
func RegisterCommand(name string, factory CommandFactory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	commandRegistry[name] = factory
}

// getCommand retrieves a command by name using the given root context.
// Returns nil, false if the command is not registered.
func getCommand(name string, root *RootCommand) (Command, bool) {
	registryMu.RLock()
	factory, ok := commandRegistry[name]
	registryMu.RUnlock()
	if !ok {
		return nil, false
	}
	return factory(root), true
}

// RegisteredCommandNames returns sorted list of registered command names.
func RegisteredCommandNames() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]string, 0, len(commandRegistry))
	for name := range commandRegistry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// CommandDispatcher dispatches a command with config and database context.
type CommandDispatcher struct {
	cfg *config.Config
	db  database.DatabaseManager
}

// NewCommandDispatcher creates a new dispatcher.
func NewCommandDispatcher(cfg *config.Config, db database.DatabaseManager) *CommandDispatcher {
	return &CommandDispatcher{cfg: cfg, db: db}
}

// Dispatch dispatches a command by constructing it with the injected context.
func (d *CommandDispatcher) Dispatch(cmdName string, args []string) int {
	root := &RootCommand{cfg: d.cfg, db: d.db}

	registryMu.RLock()
	factory, ok := commandRegistry[cmdName]
	registryMu.RUnlock()
	if !ok {
		return 127
	}

	cmd := factory(root)
	return cmd.Run(args)
}
