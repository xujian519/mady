package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/xujian519/mady/mcp"
)

// runMCPInstall handles the "mady mcp-install" subcommand.
// Usage:
//
//	mady mcp-install <agent>          install Mady MCP server into <agent>
//	mady mcp-install --list            list detected agents
//	mady mcp-install <agent> --print   dry-run: show what would be written
//	mady mcp-install <agent> --uninstall  remove Mady from <agent> config
func runMCPInstall(ctx context.Context, args []string) error {
	// Parse flags.
	var listFlag, printFlag, uninstallFlag bool
	var agentArg string
	for _, arg := range args {
		switch arg {
		case "--list", "-l":
			listFlag = true
		case "--print", "-p":
			printFlag = true
		case "--uninstall", "-u":
			uninstallFlag = true
		case "--help", "-h":
			printMCPInstallUsage()
			return nil
		default:
			if !strings.HasPrefix(arg, "-") && agentArg == "" {
				agentArg = arg
			}
		}
	}

	if listFlag {
		return listAgents()
	}

	if agentArg == "" {
		printMCPInstallUsage()
		return fmt.Errorf("mcp-install: agent argument is required")
	}

	if uninstallFlag {
		result, err := mcp.UninstallMady(agentArg, printFlag)
		if err != nil {
			return err
		}
		printResult(result)
		return nil
	}

	result, err := mcp.InstallMady(agentArg, printFlag)
	if err != nil {
		return err
	}
	printResult(result)
	return nil
}

func listAgents() error {
	agents := mcp.DetectAgents()
	if len(agents) == 0 {
		fmt.Println("No coding agents detected on this system.")
		return nil
	}
	fmt.Println("Detected coding agents:")
	fmt.Println()
	for _, a := range agents {
		status := "not detected"
		if a.Installed {
			status = "installed  "
		}
		fmt.Printf("  [%s] %-15s (%s)\n", status, a.Name, a.DisplayName)
		fmt.Printf("        config: %s\n", a.ConfigPath)
	}
	fmt.Println()
	fmt.Println("Install Mady via: mady mcp-install <agent>")
	return nil
}

func printResult(result *mcp.InstallResult) {
	if result.Error != nil {
		fmt.Fprintf(os.Stderr, "Note: %v\n", result.Error)
	}
	if result.DryRun {
		fmt.Printf("[DRY RUN] Would %s Mady for %s\n", result.Action, result.Agent)
		fmt.Printf("Config file: %s\n", result.ConfigPath)
		if result.Preview != "" {
			fmt.Printf("Preview:\n%s\n", result.Preview)
		}
		return
	}
	fmt.Printf("Successfully %sed Mady MCP server for %s.\n", result.Action, result.Agent)
	fmt.Printf("Config file: %s\n", result.ConfigPath)
}

func printMCPInstallUsage() {
	fmt.Fprintln(os.Stderr, `mady mcp-install — wire Mady as an MCP server into coding agents

Usage:
  mady mcp-install <agent>           install Mady into <agent>
  mady mcp-install <agent> --print   dry-run without writing to disk
  mady mcp-install <agent> --uninstall  remove Mady from <agent>
  mady mcp-install --list            list detected coding agents

Supported agents:
  claude    Claude Code
  codex     Codex CLI
  cursor    Cursor
  gemini    Gemini CLI
  copilot   GitHub Copilot

Examples:
  mady mcp-install claude
  mady mcp-install codex --print
  mady mcp-install --list`)
}
