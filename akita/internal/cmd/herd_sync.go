package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/sarchlab/akita/v5/herd"
	"github.com/sarchlab/akita/v5/herd/backend"
	"github.com/spf13/cobra"
)

var herdSyncCmd = &cobra.Command{
	Use:   "sync [host...]",
	Short: "Copy the project to hosts (all hosts by default).",
	Long: `Copy the project to <root>/<project> on hosts with rsync --delete, so the
copy matches the project. Not copied (and never deleted there): .git/,
logs/, results/, remote/, __pycache__/, .DS_Store, .herd_archive.json and
.exp_archive.json, the paths the project's .gitignore files ignore,
herd.json sync.exclude, and the patterns in .herdignore and .expignore.
The local host without a root uses the project directory itself and is
skipped.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		env, err := loadHerdEnv(cmd)
		if err != nil {
			return err
		}

		bs, err := env.backends(args)
		if err != nil {
			return err
		}

		ctx, cancel := commandContext(cmd)
		defer cancel()

		for _, b := range bs {
			if err := syncTo(ctx, env, b); err != nil {
				return err
			}
		}

		return nil
	},
}

var herdPullCmd = &cobra.Command{
	Use:   "pull [host...]",
	Short: "Copy logs/ and results/ of hosts to remote/<host>/.",
	RunE: func(cmd *cobra.Command, args []string) error {
		env, err := loadHerdEnv(cmd)
		if err != nil {
			return err
		}

		bs, err := env.backends(args)
		if err != nil {
			return err
		}

		ctx, cancel := commandContext(cmd)
		defer cancel()

		for _, b := range bs {
			if env.isProjectWorkspace(b) {
				continue
			}

			dir := filepath.Join(env.projectDir, "remote", b.Host().Name)
			if err := b.Pull(ctx, env.project, dir); err != nil {
				return err
			}

			env.printf(">> pulled %s -> remote/%s/\n", b.Host().Name, b.Host().Name)
		}

		return nil
	},
}

var herdInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Write ./herd.json and the user config if they do not exist.",
	Long: `Write ./herd.json and the user config if they do not exist. A project
or user config under its former name (exp.json) counts as existing.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}

		project := &herd.ProjectConfig{
			Schema: herd.SchemaProject,
			Sync:   herd.SyncConfig{Exclude: []string{"remote/", "results/", "logs/"}},
		}
		if err := writeIfMissing(cmd, herd.ProjectConfigPath(dir), project); err != nil {
			return err
		}

		user := &herd.UserConfig{Hosts: []herd.HostConfig{
			{Name: "local", Type: herd.HostLocal},
		}}

		return writeIfMissing(cmd, herd.UserConfigPath(), user)
	},
}

func init() {
	herdCmd.AddCommand(herdSyncCmd, herdPullCmd, herdInitCmd)
}

// syncTo syncs the project to a host unless its workspace is the project.
func syncTo(ctx context.Context, env *herdEnv, b backend.Backend) error {
	if env.isProjectWorkspace(b) {
		return nil
	}

	env.printf(">> sync %s -> %s:%s\n", env.project, b.Host().Name, b.Workspace(env.project))

	return b.Sync(ctx, env.projectDir, env.projectCfg)
}

func writeIfMissing(cmd *cobra.Command, path string, v any) error {
	if _, err := os.Stat(path); err == nil {
		fmt.Fprintf(cmd.OutOrStdout(), "%s exists, left unchanged\n", path)
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", path)

	return nil
}
