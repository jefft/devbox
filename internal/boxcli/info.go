// Copyright 2024 Jetify Inc. and contributors. All rights reserved.
// Use of this source code is governed by the license in the LICENSE file.

package boxcli

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"go.jetify.com/devbox/internal/boxcli/usererr"
	"go.jetify.com/devbox/internal/devbox"
	"go.jetify.com/devbox/internal/devbox/devopt"
)

type infoCmdFlags struct {
	config   configFlags
	markdown bool
	json     bool
}

func infoCmd() *cobra.Command {
	flags := infoCmdFlags{}
	command := &cobra.Command{
		Use:   "info [<pkg>]",
		Short: "Display package info, or resolved project paths when no package is given",
		Args:  cobra.MaximumNArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Project paths are a pure function of the config tree; only
			// package info needs nix.
			if len(args) == 1 {
				return ensureNixInstalled(cmd, args)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				if flags.json {
					return usererr.New("--json is only supported without a package argument")
				}
				return infoCmdFunc(cmd, args[0], flags)
			}
			return projectInfoCmdFunc(cmd, flags)
		},
	}

	flags.config.register(command)
	command.Flags().BoolVar(&flags.markdown, "markdown", false, "output in markdown format")
	command.Flags().BoolVar(&flags.json, "json", false, "output in JSON format (only without a package argument)")
	return command
}

func projectInfoCmdFunc(cmd *cobra.Command, flags infoCmdFlags) error {
	box, err := devbox.Open(&devopt.Opts{
		Dir:         flags.config.path,
		Environment: flags.config.environment,
		Stderr:      cmd.ErrOrStderr(),
	})
	if err != nil {
		return errors.WithStack(err)
	}

	paths := box.ProjectPaths()
	if flags.json {
		out, err := json.MarshalIndent(paths, "", "  ")
		if err != nil {
			return errors.WithStack(err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(out))
		return nil
	}

	writer := tabwriter.NewWriter(cmd.OutOrStdout(), 3, 2, 8, ' ', tabwriter.TabIndent)
	fmt.Fprintf(writer, "Project dir:\t%s\n", paths.ProjectDir)
	fmt.Fprintf(writer, "Devbox dir root:\t%s\n", paths.DevboxDirRoot)
	fmt.Fprintf(writer, "Profile:\t%s\n", paths.ProfilePath)
	for _, inc := range paths.Includables {
		fmt.Fprintf(writer, "\n%s:\n", inc.Name)
		fmt.Fprintf(writer, "  devbox dir:\t%s\n", inc.DevboxDir)
		fmt.Fprintf(writer, "  virtenv:\t%s\n", inc.Virtenv)
		fmt.Fprintf(writer, "  data:\t%s\n", inc.DataDir)
		fmt.Fprintf(writer, "  log:\t%s\n", inc.LogDir)
		fmt.Fprintf(writer, "  runtime:\t%s\n", inc.RuntimeDir)
	}
	return writer.Flush()
}

func infoCmdFunc(cmd *cobra.Command, pkg string, flags infoCmdFlags) error {
	box, err := devbox.Open(&devopt.Opts{
		Dir:         flags.config.path,
		Environment: flags.config.environment,
		Stderr:      cmd.ErrOrStderr(),
	})
	if err != nil {
		return errors.WithStack(err)
	}

	info, err := box.Info(cmd.Context(), pkg, flags.markdown)
	if err != nil {
		return errors.WithStack(err)
	}
	fmt.Fprint(cmd.OutOrStdout(), info)
	return nil
}
