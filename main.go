package main

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/IanS5/go-proj/repo"
	"github.com/IanS5/go-proj/service"

	log "github.com/sirupsen/logrus"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

var cacheFilePath = path.Join(os.Getenv("HOME"), ".proj-cache")

var (
	ErrNoResticRepos       = errors.New("No restic repos found, please specify $PROJ_RESTIC_REPOS")
	ErrResticNotFound      = errors.New("Could not find restic in $PATH")
	ErrOptionOutOfRange    = errors.New("Invalid response, please choose one of the provided options")
	ErrMissingDropboxToken = errors.New("No dropbox token found, please set $PROJ_DROPBOX_TOKEN")
)

type CachedRepo struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type Cache struct {
	Repos       []CachedRepo `json:"repos"`
	PrimaryRepo string       `json:"primary-repo"`
	DropboxKey  string       `json:"dropbox-key"`
}

func getCache() *Cache {
	if data, err := ioutil.ReadFile(cacheFilePath); err != nil {
		cache := &Cache{
			Repos: make([]CachedRepo, 0),
		}

		writeCache(cache)
		return cache
	} else {
		cache := &Cache{}
		err = json.Unmarshal(data, cache)
		if err != nil {
			log.Fatal(err)
		}
		return cache
	}
}

func writeCache(c *Cache) {
	log.WithField("Cache", c).Debug("Writing cache")

	f, err := os.Create(cacheFilePath)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	data, err := json.Marshal(c)
	if err != nil {
		log.Fatal(err)
	}

	f.Write(data)
	f.Sync()
}

func validateName(p string) {
	if !regexp.MustCompile("[a-zA-Z0-9+_\\-!]*").MatchString(p) {
		log.WithField("Name", p).Fatal("Repo names may only contain letters, numbers, _, - and !")
	}
}

func (cache *Cache) parseStorageService(p string) service.StorageService {
	switch strings.Trim(strings.ToLower(p), "\n\r\t\v ") {
	case "dropbox":
		if cache.DropboxKey == "" {
			log.Fatal("You have not logged into dropbox")
		}
		return service.NewDropbox(cache.DropboxKey)
	}
	return nil
}

func (cache *Cache) parseProject(p string) (project string, repository *repo.ProjectRepository) {
	parts := strings.Split(p, "/")
	if len(parts) > 2 {
		log.Fatal("Projects should be identifier with either [repo]/[project] or simply [project]")
	}

	var repoName string
	if len(parts) == 1 {
		if cache.PrimaryRepo == "" {
			log.Fatal("No primary repo set!")
		}

		repoName = cache.PrimaryRepo
		project = parts[0]
	} else {
		repoName = parts[0]
		project = parts[1]
	}

	validateName(repoName)
	validateName(project)

	var folder string
	for _, r := range cache.Repos {
		if r.Name == repoName {
			folder = r.Path
			break
		}
	}

	if folder == "" {
		log.Fatalf("No such repo \"%s\"", repoName)
	}

	return project, repo.NewInteractiveLocal(folder)
}

func addProjectOp(app *kingpin.Application, project *string, long, short, help string) {
	app.Command(long, help).
		Alias(short).
		Arg("PROJECT", "Project to operate on, in the form of [repo]/[project]").
		Required().
		NoEnvar().
		StringVar(project)
}

func main() {
	log.SetFormatter(&log.TextFormatter{
		DisableTimestamp:       true,
		DisableLevelTruncation: true,
		DisableSorting:         true,
		QuoteEmptyFields:       true,
	})
	log.SetOutput(os.Stdout)
	log.SetLevel(log.DebugLevel)

	app := kingpin.
		New("proj", "A stupid simple project management CLI").
		Author("Ian Shehadeh <IanShehadeh2020@gmail.com>").
		Version("0.1.1")

	newRepo := app.Command("init", "initialize a new repository")
	newRepoName := newRepo.Arg("NAME", "New repository name").Required().NoEnvar().String()
	newRepoDir := newRepo.Arg("DIR", "New repository location").Required().NoEnvar().ExistingDir()

	setPrimary := app.Command("primary", "set a new repo as the primary one")
	setPrimaryName := setPrimary.Arg("NAME", "The primary repository's name").Required().NoEnvar().String()

	db := app.Command("dropbox", "login/logoff from dropbox")
	db.Command("login", "Login to dropbox")
	db.Command("logout", "Logout of dropbox")

	var project string
	var serviceName string

	addProjectOp(app, &project, "visit", "v", "Visit a project")
	addProjectOp(app, &project, "create", "c", "Create a new project")
	addProjectOp(app, &project, "remove", "r", "Remove a project")

	upload := app.Command("upload", "Upload a local project to a remote storage service").Alias("u")
	upload.Arg("PROJECT", "Project to operate on, in the form of [repo]/[project]").
		Required().
		NoEnvar().
		StringVar(&project)
	upload.Arg("SERVICE", "The service where the project should be uploaded").
		Default("dropbox").
		NoEnvar().
		EnumVar(&serviceName, "dropbox")

	download := app.Command("download", "Download a remote packcage").Alias("d")
	download.Arg("PROJECT", "Project to operate on, in the form of [repo]/[project]").
		Required().
		NoEnvar().
		StringVar(&project)
	download.Arg("SERVICE", "The service where the project should be uploaded").
		Default("dropbox").
		NoEnvar().
		EnumVar(&serviceName, "dropbox")

	cmd, err := app.Parse(os.Args[1:])
	log.Debug("Command ", cmd)
	if err != nil {
		log.Fatal(err)
	}

	cache := getCache()
	if cmd == "init" {
		log.Infof("Adding repo %s", *newRepoName)
		validateName(*newRepoName)

		cache.Repos = append(cache.Repos, CachedRepo{
			Name: *newRepoName,
			Path: *newRepoDir,
		})

		writeCache(cache)
		return
	}

	if cmd == "dropbox login" {
		cache.DropboxKey, err = service.InteractiveDropboxLogin()
		if err != nil {
			log.Fatal(err)
		}
		writeCache(cache)
	}

	if cmd == "dropbox logout" {
		err = service.DropboxInvalidateToken(cache.DropboxKey)
		if err != nil {
			log.Fatal(err)
		}
		cache.DropboxKey = ""
		writeCache(cache)
	}

	proj, repo := cache.parseProject(project)

	if cmd == "primary" {
		log.Infof("Setting repo \"%s\" as the primary repository", *setPrimaryName)

		validateName(*setPrimaryName)
		exists := false
		for _, r := range cache.Repos {
			if r.Name == *setPrimaryName {
				exists = true
			}
		}
		if !exists {
			log.Fatalf("repo \"%s\" does not exist", *setPrimaryName)

			cache.PrimaryRepo = *setPrimaryName
			writeCache(cache)
			return
		}
	}

	switch cmd {
	case "visit":
		err = repo.Visit(proj)

	case "create":
		err = repo.Create(proj)

	case "remove":
		err = repo.Delete(proj)

	case "upload":
		err = repo.Upload(proj, cache.parseStorageService(serviceName))

	case "download":
		err = repo.Pull(proj, cache.parseStorageService(serviceName))
	}

	if err != nil {
		log.Fatal(err)
	}
}
