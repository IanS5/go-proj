package cmd

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var cmdResticAdd = &cobra.Command{
	Use:   "add REPO",
	Short: "Add a new restic repo",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		config.Restic.Repositories = append(config.Restic.Repositories, args[0])
		config.Write()
	},
}

var cmdResticList = &cobra.Command{
	Use:   "list",
	Short: "List restic repositories",
	Run: func(cmd *cobra.Command, args []string) {
		for _, name := range config.Restic.Repositories {
			fmt.Println(name)
		}
	},
}

var cmdResticRemove = &cobra.Command{
	Use:   "remove REPO",
	Short: "Remove a restic repo",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		idx := -1
		for i, repo := range config.Restic.Repositories {
			if repo == args[0] {
				idx = i
				logrus.WithField("Index", idx).Debug("Found repo")
			}
		}

		if idx == -1 {
			logrus.Fatal("Repo not found")
		}

		config.Restic.Repositories =
			append(config.Restic.Repositories[:idx], config.Restic.Repositories[idx+1:]...)

		config.Write()
	},
}

var cmdRestic = &cobra.Command{
	Use:   "restic",
	Short: "Manage restic repositories",
}

func init() {
	cmdRestic.AddCommand(cmdResticAdd, cmdResticRemove, cmdResticList)
}
