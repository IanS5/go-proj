package cmd

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var cmdRepo = &cobra.Command{
	Use:   "repo",
	Short: "Manage your the dropbox account associated with proj",
}

var cmdRepoList = &cobra.Command{
	Use:   "list",
	Short: "List repositories, and where they point",
	Run: func(cmd *cobra.Command, args []string) {
		for name, folder := range config.ProjectRepositories {
			fmt.Printf("%s %s\n", name, folder)
		}
	},
}

var cmdRepoAdd = &cobra.Command{
	Use:   "add NAME PATH",
	Short: "Add a repository",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		if config.ProjectRepositories == nil {
			config.ProjectRepositories = make(map[string]string)
		}

		config.ProjectRepositories[args[0]] = args[1]
		config.Write()
	},
}

var cmdRepoRemove = &cobra.Command{
	Use:   "remove",
	Short: "Remove a project repository (this doesn't actually delete the folder)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if config.PrimaryRepo == args[0] {
			logrus.Debug("Resetting primary repo, because it was removed")
			config.PrimaryRepo = ""
		}

		delete(config.ProjectRepositories, args[0])
		config.Write()
	},
}

var cmdRepoPrimary = &cobra.Command{
	Use:   "primary REPO",
	Short: "Set the primary repository",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if _, exists := config.ProjectRepositories[args[0]]; !exists {
			logrus.
				WithField("Repo", args[0]).
				Fatal("repository does not exist")
		}

		config.PrimaryRepo = args[0]
		config.Write()
	},
}

func init() {
	cmdRepo.AddCommand(cmdRepoAdd, cmdRepoList, cmdRepoPrimary, cmdRepoRemove)
}
