// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package main is the entry point for the MCP Toolbox server.
// MCP Toolbox is a server that exposes tools and data sources via the
// Model Context Protocol (MCP), enabling AI models to interact with
// databases and other data sources.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/config"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	// version is set at build time via ldflags.
	version = "dev"
	// commit is the git commit hash set at build time.
	commit = "unknown"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var cfg config.Config

	rootCmd := &cobra.Command{
		Use:   "mcp-toolbox",
		Short: "MCP Toolbox — expose tools and data sources via the Model Context Protocol",
		Long: `MCP Toolbox is a server that implements the Model Context Protocol (MCP).
It allows AI models and agents to securely interact with databases,
APIs, and other data sources through a standardized interface.`,
		Version: fmt.Sprintf("%s (commit: %s)", version, commit),
		RunE: func(cmd *cobra.Command, args []string) error {
			return startServer(cmd.Context(), &cfg)
		},
	}

	// Register persistent flags.
	flags := rootCmd.PersistentFlags()
	// Default config file name changed to my-tools.yaml so it doesn't
	// accidentally pick up the upstream example file when testing locally.
	flags.StringVar(&cfg.ConfigFile, "tools-file", "my-tools.yaml", "Path to the tools configuration file")
	// Bind to localhost by default for better security; use 0.0.0.0 explicitly
	// if you need the server reachable on the local network or from Docker.
	flags.StringVar(&cfg.Address, "address", "127.0.0.1", "Address to bind the server to")
	// Changed default port from 5000 to 5001 to avoid conflict with AirPlay
	// Receiver on macOS, which also listens on port 5000.
	flags.IntVar(&cfg.Port, "port", 5001, "Port to listen on")
	flags.BoolVar(&cfg.LogJSON, "log-json", false, "Output logs in JSON format")
	// Enable debug logging by default locally — makes it easier to trace tool
	// calls during development without having to remember the flag every time.
	flags.BoolVar(&cfg.Debug, "debug", true, "Enable debug logging")

	// Set up context with signal handling for graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return rootCmd.ExecuteContext(ctx)
}

// startServer initialise