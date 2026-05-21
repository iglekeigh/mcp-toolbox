// Copyright 2026 Google LLC
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

package invoke

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/googleapis/mcp-toolbox/cmd/internal"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/server/resources"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func NewCommand(opts *internal.ToolboxOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "invoke <tool-name> [flags]",
		Short: "Execute a tool directly",
		Long: `Execute a tool directly with parameters.
Parameters can be passed as flags or as a JSON string.
Example:
  toolbox invoke my-tool --param1 value1
  toolbox invoke my-tool '{"param1": "value1"}'`,
		Args:               cobra.MinimumNArgs(1),
		DisableFlagParsing: false,
		RunE: func(c *cobra.Command, args []string) error {
			return runInvoke(c, args, opts)
		},
	}
	flags := cmd.Flags()
	flags.SetInterspersed(false)
	internal.ConfigFileFlags(flags, opts)
	return cmd
}

func runInvoke(cmd *cobra.Command, args []string, opts *internal.ToolboxOptions) error {
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	if !cmd.Flags().Changed("log-level") {
		_ = opts.Cfg.LogLevel.Set("warn")
	}

	ctx, shutdown, err := opts.Setup(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = shutdown(ctx)
	}()

	// With SetInterspersed(false), args[0] is the tool name and args[1:] are the tool arguments.
	toolArgs := args[1:]

	_, err = opts.LoadConfig(ctx, &internal.ConfigParser{})
	if err != nil {
		return err
	}

	// Initialize Resources
	sourcesMap, authServicesMap, embeddingModelsMap, toolsMap, toolsetsMap, promptsMap, promptsetsMap, err := server.InitializeConfigs(ctx, opts.Cfg)
	if err != nil {
		errMsg := fmt.Errorf("failed to initialize resources: %w", err)
		opts.Logger.ErrorContext(ctx, errMsg.Error())
		return errMsg
	}

	resourceMgr := resources.NewResourceManager(sourcesMap, authServicesMap, embeddingModelsMap, toolsMap, toolsetsMap, promptsMap, promptsetsMap)

	// Execute Tool
	toolName := args[0]
	tool, ok := resourceMgr.GetTool(toolName)
	if !ok {
		errMsg := fmt.Errorf("tool %q not found", toolName)
		opts.Logger.ErrorContext(ctx, errMsg.Error())
		return errMsg
	}

	// Check if the user requested help for the specific tool
	for _, arg := range toolArgs {
		if arg == "--help" || arg == "-h" {
			fmt.Printf("Usage: toolbox invoke %s [flags]\n", toolName)
			if tool.Manifest().Description != "" {
				fmt.Println(tool.Manifest().Description)
			}
			fmt.Println("\nFlags:")
			fs := pflag.NewFlagSet(toolName, pflag.ContinueOnError)
			registerDynamicFlags(fs, tool.GetParameters())
			fmt.Print(fs.FlagUsages())
			return nil // Exit early
		}
	}

	params := make(map[string]any)
	if len(toolArgs) > 0 {
		// Fallback to JSON string if only one argument and it looks like JSON
		if len(toolArgs) == 1 && strings.HasPrefix(toolArgs[0], "{") {
			if err := util.DecodeJSON(strings.NewReader(toolArgs[0]), &params); err != nil {
				errMsg := fmt.Errorf("params must be a valid JSON string: %w", err)
				opts.Logger.ErrorContext(ctx, errMsg.Error())
				return errMsg
			}
		} else {
			var err error
			params, err = parseDynamicFlags(toolName, tool.GetParameters(), toolArgs)
			if err != nil {
				opts.Logger.ErrorContext(ctx, err.Error())
				return err
			}
		}
	}

	parsedParams, err := parameters.ParseParams(tool.GetParameters(), params, nil)
	if err != nil {
		errMsg := fmt.Errorf("invalid parameters: %w", err)
		opts.Logger.ErrorContext(ctx, errMsg.Error())
		return errMsg
	}

	parsedParams, err = tool.EmbedParams(ctx, parsedParams, resourceMgr.GetEmbeddingModelMap())
	if err != nil {
		errMsg := fmt.Errorf("error embedding parameters: %w", err)
		opts.Logger.ErrorContext(ctx, errMsg.Error())
		return errMsg
	}

	// Client Auth not supported for ephemeral CLI call
	requiresAuth, err := tool.RequiresClientAuthorization(resourceMgr)
	if err != nil {
		errMsg := fmt.Errorf("failed to check auth requirements: %w", err)
		opts.Logger.ErrorContext(ctx, errMsg.Error())
		return errMsg
	}
	if requiresAuth {
		errMsg := fmt.Errorf("client authorization is not supported")
		opts.Logger.ErrorContext(ctx, errMsg.Error())
		return errMsg
	}

	result, err := tool.Invoke(ctx, resourceMgr, parsedParams, "")
	if err != nil {
		errMsg := fmt.Errorf("tool execution failed: %w", err)
		opts.Logger.ErrorContext(ctx, errMsg.Error())
		return errMsg
	}

	// Print Result
	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		errMsg := fmt.Errorf("failed to marshal result: %w", err)
		opts.Logger.ErrorContext(ctx, errMsg.Error())
		return errMsg
	}
	fmt.Fprintln(opts.IOStreams.Out, string(output))

	return nil
}

// registerDynamicFlags registers tool parameters as flags on the given FlagSet.
func registerDynamicFlags(fs *pflag.FlagSet, toolParams []parameters.Parameter) (
	stringPointers map[string]*string,
	intPointers map[string]*int,
	boolPointers map[string]*bool,
	floatPointers map[string]*float64,
	stringSlicePointers map[string]*[]string,
	mapPointers map[string]*string,
) {
	stringPointers = make(map[string]*string)
	intPointers = make(map[string]*int)
	boolPointers = make(map[string]*bool)
	floatPointers = make(map[string]*float64)
	stringSlicePointers = make(map[string]*[]string)
	mapPointers = make(map[string]*string)

	for _, p := range toolParams {
		name := p.GetName()
		desc := p.Manifest().Description
		if p.Manifest().Required {
			desc = "[Required] " + desc
		} else {
			desc = "[Optional] " + desc
		}

		switch p.GetType() {
		case parameters.TypeString:
			stringPointers[name] = fs.String(name, "", desc)
		case parameters.TypeInt:
			intPointers[name] = fs.Int(name, 0, desc)
		case parameters.TypeBool:
			boolPointers[name] = fs.Bool(name, false, desc)
		case parameters.TypeFloat:
			floatPointers[name] = fs.Float64(name, 0.0, desc)
		case parameters.TypeArray:
			stringSlicePointers[name] = fs.StringSlice(name, nil, desc)
		case parameters.TypeMap:
			mapPointers[name] = fs.String(name, "", desc)
		}
	}
	return
}

func parseDynamicFlags(toolName string, toolParams []parameters.Parameter, toolArgs []string) (map[string]any, error) {
	params := make(map[string]any)
	fs := pflag.NewFlagSet(toolName, pflag.ContinueOnError)
	fs.Usage = func() {} // Disable default usage printing

	stringPointers, intPointers, boolPointers, floatPointers, stringSlicePointers, mapPointers := registerDynamicFlags(fs, toolParams)

	if err := fs.Parse(toolArgs); err != nil {
		return nil, fmt.Errorf("failed to parse arguments: %w", err)
	}

	// Collect only changed flags to preserve defaults in ParseParams
	for name, ptr := range stringPointers {
		if fs.Changed(name) {
			params[name] = *ptr
		}
	}
	for name, ptr := range intPointers {
		if fs.Changed(name) {
			params[name] = *ptr
		}
	}
	for name, ptr := range boolPointers {
		if fs.Changed(name) {
			params[name] = *ptr
		}
	}
	for name, ptr := range floatPointers {
		if fs.Changed(name) {
			params[name] = *ptr
		}
	}
	for name, ptr := range stringSlicePointers {
		if fs.Changed(name) {
			// Find the parameter to know its item type
			var targetParam parameters.Parameter
			for _, p := range toolParams {
				if p.GetName() == name {
					targetParam = p
					break
				}
			}

			var anySlice []any
			if targetParam != nil {
				if arrayParam, ok := targetParam.(*parameters.ArrayParameter); ok {
					itemType := arrayParam.Items.GetType()
					anySlice = make([]any, len(*ptr))
					for i, val := range *ptr {
						switch itemType {
						case parameters.TypeInt:
							if intVal, err := strconv.Atoi(val); err == nil {
								anySlice[i] = intVal
							} else {
								return nil, fmt.Errorf("failed to convert array element %q to integer: %w", val, err)
							}
						case parameters.TypeFloat:
							if floatVal, err := strconv.ParseFloat(val, 64); err == nil {
								anySlice[i] = floatVal
							} else {
								return nil, fmt.Errorf("failed to convert array element %q to float: %w", val, err)
							}
						case parameters.TypeBool:
							if boolVal, err := strconv.ParseBool(val); err == nil {
								anySlice[i] = boolVal
							} else {
								return nil, fmt.Errorf("failed to convert array element %q to boolean: %w", val, err)
							}
						default:
							anySlice[i] = val
						}
					}
				}
			}
			if anySlice == nil {
				// Fallback or if type was string
				anySlice = make([]any, len(*ptr))
				for i, val := range *ptr {
					anySlice[i] = val
				}
			}
			params[name] = anySlice
		}
	}
	for name, ptr := range mapPointers {
		if fs.Changed(name) && *ptr != "" {
			var mapVal map[string]any
			if err := util.DecodeJSON(strings.NewReader(*ptr), &mapVal); err != nil {
				return nil, fmt.Errorf("failed to parse map parameter %q as JSON: %w", name, err)
			}
			params[name] = mapVal
		}
	}

	return params, nil
}
