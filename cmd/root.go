package cmd

import (
	"fmt"
	"log"
	"os"
	"strings"

	proj "github.com/IanS5/go-proj"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var config *proj.Config
var debug = false
var storageServiceName = ""
var showRepoList = false

func parseStorageService(service string) proj.StorageService {
	switch strings.Trim(strings.ToLower(storageServiceName), "\t\r\n\v ") {
	case "dropbox":
		return proj.NewDropbox(config.Dropbox.Token)
	default:
		logrus.
			WithField("Service", storageServiceName).
			WithField("Options", []string{"dropbox"}).
			Fatal("invalid storage service")
	}
	return nil
}

func makeProjectAction(name string, description string, action func(repo *proj.ProjectRepository, project string) error) (cmd *cobra.Command) {
	var repo string

	cmd = &cobra.Command{
		Use:     name + " PROJECT",
		Short:   description,
		Args:    cobra.ExactArgs(1),
		Aliases: []string{string(name[0])},
		Run: func(cmd *cobra.Command, args []string) {
			if repo == "" {
				repo = config.PrimaryRepo
			}

			repoPath, exists := config.ProjectRepositories[repo]
			if !exists {
				logrus.WithField("Repo", repo).Fatal("repository does not exist")
			}

			err := action(proj.NewInteractiveLocal(repoPath), args[0])
			if err != nil {
				log.Fatal(err)
			}
		},
	}

	cmd.PersistentFlags().StringVarP(&repo, "repo", "r", "", "Repo where the project is located, or the primary repo if this flag is omitted")
	return
}

var cmdRoot = &cobra.Command{
	Use:   "proj",
	Short: "Project manager",
	Long:  `An application to add convient functionality ontop of your existing filesystem`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if debug {
			logrus.SetLevel(logrus.DebugLevel)
			logrus.Debug("Debugging mode enabled")
		} else {
			logrus.SetLevel(logrus.InfoLevel)
		}
	},
}

var cmdVisit = makeProjectAction("visit",
	"Open a new instance of your shell in this project's directory",
	func(repo *proj.ProjectRepository, project string) error {
		return repo.Visit(project)
	})

var cmdCreate = makeProjectAction("create",
	"Create a new project",
	func(repo *proj.ProjectRepository, project string) error {
		return repo.Create(project)
	})

var cmdRemove = makeProjectAction("remove",
	"Remove a project",
	func(repo *proj.ProjectRepository, project string) error {
		return repo.Delete(project)
	})

var cmdUpload = makeProjectAction("upload",
	"Upload a project to a storage service",
	func(repo *proj.ProjectRepository, project string) error {
		return repo.Upload(project, parseStorageService(storageServiceName))
	})

var cmdDownload = makeProjectAction("download",
	"Download a project from a storage service",
	func(repo *proj.ProjectRepository, project string) error {
		return repo.Pull(project, parseStorageService(storageServiceName))
	})

var cmdList = &cobra.Command{
	Use:     "list FILTERS...",
	Short:   "List projects filtered by a 0 or more regular expressions",
	Aliases: []string{"l"},
	Run: func(cmd *cobra.Command, args []string) {
		for name, path := range config.ProjectRepositories {
			if projects, err := proj.NewLocal(path).List(args...); err == nil {
				for _, project := range projects {
					if showRepoList {
						fmt.Printf("%s %s\n", name, project)
					} else {
						fmt.Println(project)

					}
				}
			} else {
				logrus.WithField("Repo", name).Fatal(err)
			}
		}
		return
	},
}

func init() {
	logrus.SetFormatter(&logrus.TextFormatter{
		DisableTimestamp:       true,
		DisableLevelTruncation: true,
		QuoteEmptyFields:       true,
		DisableSorting:         true,
	})
	cmdList.PersistentFlags().BoolVarP(&showRepoList, "show-repo", "w", false, "Show the repo each project comes from")
	cmdUpload.PersistentFlags().StringVarP(&storageServiceName, "service", "s", "", "The service where the project will be uploaded")
	cmdDownload.PersistentFlags().StringVarP(&storageServiceName, "service", "s", "", "The service where the project can be downloaded")

	cmdRoot.PersistentFlags().BoolVarP(&debug, "debug", "d", false, "Show debugging information")
	cmdRoot.AddCommand(
		cmdDropbox,
		cmdRestic,
		cmdUpload,
		cmdList,
		cmdDownload,
		cmdVisit,
		cmdRepo,
		cmdCreate,
		cmdRemove)
}

func Execute() {
	config = proj.LoadConfig()
	if err := cmdRoot.Execute(); err != nil {
		os.Exit(1)
	}
}
