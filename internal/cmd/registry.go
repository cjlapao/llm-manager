// Package cmd provides the command registry for the CLI application.
package cmd

import (
	"sort"

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
var commandRegistry = make(map[string]CommandFactory)

// RegisterCommand adds a command factory to the global registry.
func RegisterCommand(name string, factory CommandFactory) {
	commandRegistry[name] = factory
}

// GetCommand retrieves a command by name using the given root context.
// Returns nil, false if the command is not registered.
func GetCommand(name string, root *RootCommand) (Command, bool) {
	factory, ok := commandRegistry[name]
	if !ok {
		return nil, false
	}
	return factory(root), true
}

// RegisteredCommandNames returns sorted list of registered command names.
func RegisteredCommandNames() []string {
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

	factory, ok := commandRegistry[cmdName]
	if !ok {
		return 1
	}

	cmd := factory(root)
	return cmd.Run(args)
}
