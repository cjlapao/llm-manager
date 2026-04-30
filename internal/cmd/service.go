// Package cmd provides the service subcommand for llm-manager.
package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/user/llm-manager/internal/service"
)

func init() {
	RegisterCommand("service", func(root *RootCommand) Command { return NewServiceCommand(root) })
}

// ServiceCommand provides high-level service orchestration.
type ServiceCommand struct {
	cfg *RootCommand
	svc *service.ServiceService
}

// NewServiceCommand creates a new ServiceCommand.
func NewServiceCommand(root *RootCommand) *ServiceCommand {
	return &ServiceCommand{
		cfg: root,
		svc: service.NewServiceService(root.db, root.cfg),
	}
}

// Run executes the service command with the given subcommand and arguments.
func (c *ServiceCommand) Run(args []string) int {
	if len(args) == 0 {
		c.PrintHelp()
		return 0
	}

	switch args[0] {
	case "list", "ls", "status":
		return c.runList()
	case "start":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: 'start' requires a slug\n")
			return 1
		}
		return c.runStart(args[1])
	case "stop":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: 'stop' requires a slug\n")
			return 1
		}
		return c.runStop(args[1])
	case "restart":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: 'restart' requires a slug\n")
			return 1
		}
		return c.runRestart(args[1])
	case "help", "-h", "--help":
		c.PrintHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown service subcommand: %s\n\n", args[0])
		c.PrintHelp()
		return 1
	}
}

// runList displays all services with their status.
func (c *ServiceCommand) runList() int {
	services, err := c.svc.ListServices()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing services: %v\n", err)
		return 1
	}

	if len(services) == 0 {
		fmt.Println("No services found. Run 'llm-manager migrate' to import models.")
		return 0
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SLUG\tTYPE\tNAME\tPORT\tCONTAINER\tSTATUS")
	fmt.Fprintln(w, "----\t----\t----\t----\t---------\t------")
	for _, s := range services {
		container := s.Container
		if container == "" {
			container = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%s\n",
			s.Slug, s.Type, s.Name, s.Port, container, s.Status)
	}
	w.Flush()

	fmt.Printf("\nTotal: %d services\n", len(services))
	return 0
}

// runStart starts a service by delegating to the llm command.
func (c *ServiceCommand) runStart(slug string) int {
	llm := NewLlmCommand(c.cfg)
	return llm.Run([]string{"start", slug})
}

// runStop stops a service by delegating to the llm command.
func (c *ServiceCommand) runStop(slug string) int {
	llm := NewLlmCommand(c.cfg)
	return llm.Run([]string{"stop", slug})
}

// runRestart restarts a service by delegating to the llm command.
func (c *ServiceCommand) runRestart(slug string) int {
	llm := NewLlmCommand(c.cfg)
	return llm.Run([]string{"restart", slug})
}

// PrintHelp prints the service command help.
func (c *ServiceCommand) PrintHelp() {
	fmt.Println(`service - High-level service orchestration (delegates to llm command).

USAGE:
  llm-manager service [SUBCOMMAND] [ARGS]

SUBCOMMANDS:
  list, ls, status    List all services and their status
  start <slug>        Start a service (delegates to llm start)
  stop <slug>         Stop a service (delegates to llm stop)
  restart <slug>      Restart a service (delegates to llm restart)

EXAMPLES:
  llm-manager service list
  llm-manager service start qwen3_6
  llm-manager service stop qwen3_6
  llm-manager service restart qwen3_6

NOTES:
  This command provides a simplified interface that delegates to llm-manager llm.
  For full control (swap, status, logs), use 'llm-manager llm' directly.`)
}
