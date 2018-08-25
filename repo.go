package proj

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"syscall"

	"github.com/sirupsen/logrus"
)

const projectFolderPerm = 0775

var (
	ErrNoSuchProject = errors.New("No such project")
)

type ProjectRepository struct {
	baseFolder  string
	interactive bool
}

func modEnviron(newVars map[string]string) []string {
	env := os.Environ()
	env2 := env
	env = env[:0]

	for _, v := range env2 {
		for newVar, newVal := range newVars {
			if len(newVar) < len(v) && strings.HasPrefix(newVar, v) && v[len(newVar)] == '=' {
				env = append(env, newVar+"="+newVal)
				delete(newVars, newVar)
				break
			}
		}
		env = append(env, v)
	}

	for newVar, newVal := range newVars {
		env = append(env, newVar+"="+newVal)
	}

	return env
}

func NewLocal(base string) *ProjectRepository {
	return &ProjectRepository{
		baseFolder:  base,
		interactive: false,
	}
}

func NewInteractiveLocal(base string) *ProjectRepository {
	return &ProjectRepository{
		baseFolder:  base,
		interactive: true,
	}
}

func (fr *ProjectRepository) Path(name string) string {
	return path.Join(fr.baseFolder, name)
}

func (fr *ProjectRepository) Id(name string) string {
	// project Ids are created using the process
	// "Project##{name-of-project}" -> Sha256 -> Hex

	hashed := sha256.Sum256([]byte("Project##" + name))
	return hex.EncodeToString(hashed[:])
}

func (fr *ProjectRepository) HistFile(name string) string {
	return path.Join(os.Getenv("HOME"), ".proj", "hist", fr.Id(name))
}

func (fr *ProjectRepository) Create(name string) (err error) {
	folder := fr.Path(name)
	logrus.Debugf("Creating \"%s\" at %s", name, folder)

	if _, err := os.Stat(folder); !os.IsNotExist(err) {
		if fr.interactive && !Confirm("%s already exists, overwrite it?", name) {
			return nil
		}

		logrus.Debugf("Removing %s", folder)
		os.RemoveAll(folder)
	}

	logrus.Debugf("Making directory %s", folder)
	return os.MkdirAll(folder, projectFolderPerm)
}

func (fr *ProjectRepository) Delete(name string) (err error) {
	folder := fr.Path(name)
	logrus.Debugf("Removing \"%s\" at %s", name, folder)

	if fr.interactive && !Confirm("Are you sure you want to delete %s?", name) {
		return nil
	}
	return os.RemoveAll(folder)
}

func (fr *ProjectRepository) Visit(name string) (err error) {
	folder := fr.Path(name)
	if _, err := os.Stat(folder); os.IsNotExist(err) {
		return ErrNoSuchProject
	} else if err != nil {
		return err
	}

	shell := os.Getenv("SHELL")
	invokedExe := shell

	if shell == "" {
		invokedExe = "sh"
		shell, err = exec.LookPath("sh")
	} else if !path.IsAbs(shell) {
		shell, err = exec.LookPath(shell)
	}
	if err != nil {
		return err
	}

	env := modEnviron(map[string]string{
		"PROJ_CURRENT_PROJECT_BASE": folder,
		"PROJ_CURRENT_PROJECT_NAME": name,
		"HISTFILE":                  fr.HistFile(name),
		"fish_history":              fr.Id(name),
	})

	err = os.Chdir(folder)
	if err != nil {
		return err
	}

	ClearScreen()
	return syscall.Exec(shell, []string{invokedExe}, env)
}

func (fr *ProjectRepository) List(filters ...string) (matches []string, err error) {
	folder := fr.Path("")
	finfo, err := ioutil.ReadDir(folder)
	if err != nil {
		return
	}

	compiledFilters := make([]*regexp.Regexp, 0, len(filters))
	matches = make([]string, 0, len(finfo))

	for _, filter := range filters {
		re, err := regexp.Compile(filter)
		if err != nil {
			return nil, err
		}
		compiledFilters = append(compiledFilters, re)
	}

	for _, f := range finfo {
		if !f.IsDir() {
			continue
		}

		itemName := f.Name()
		canWrite := true
		for _, re := range compiledFilters {
			if !re.MatchString(itemName) {
				canWrite = true
				break
			}
		}

		if canWrite {
			matches = append(matches, itemName)
		}
	}
	return
}

func (fr *ProjectRepository) NonInteractive() *ProjectRepository {
	return &ProjectRepository{
		baseFolder:  fr.baseFolder,
		interactive: false,
	}
}

func (fr *ProjectRepository) Upload(name string, s StorageService) (err error) {
	folder := fr.Path(name)
	remoteFolder := "/" + name
	if _, err := os.Stat(folder); os.IsNotExist(err) {
		return ErrNoSuchProject
	} else if err != nil {
		return err
	}

	return s.WalkDiffs(folder, remoteFolder,
		func(file string, diff DiffResult) (err error) {
			switch diff {
			case DiffResultMatch:
				// Do nothing
			case DiffResultMismatch, DiffResultOnlyExistsLocal:
				localFile := path.Join(folder, file)
				remoteFile := path.Join(remoteFolder, file)
				logrus.Debugf("(UPLOAD) %q -> %q", localFile, remoteFile)
				err = s.Upload(localFile, remoteFile)
			case DiffResultOnlyExistsRemote:
				remoteFile := path.Join(remoteFolder, file)
				logrus.Debugf("(REMOVE) %q", remoteFile)
				err = s.Delete(path.Join(remoteFolder, file))
			}

			return
		})
}

func (fr *ProjectRepository) Pull(name string, s StorageService) (err error) {
	folder := fr.Path(name)
	remoteFolder := "/" + name

	os.MkdirAll(folder, projectFolderPerm)

	return s.WalkDiffs(folder, remoteFolder,
		func(file string, diff DiffResult) (err error) {
			switch diff {
			case DiffResultMatch:
				// Do nothing
			case DiffResultMismatch, DiffResultOnlyExistsRemote:
				localFile := path.Join(folder, file)
				remoteFile := path.Join(remoteFolder, file)
				logrus.Debugf("(DOWNLOAD) %q -> %q", remoteFile, localFile)
				err = s.Download(localFile, remoteFile)
			case DiffResultOnlyExistsLocal:
				localFile := path.Join(folder, file)
				logrus.Debugf("(REMOVE) %q", localFile)
				err = os.RemoveAll(localFile)
			}

			return
		})
}

func (fr *ProjectRepository) Backup(bs BackupService, name string, repos ...string) (err error) {
	return bs.Backup(fr.Path(name), repos...)
}

func (fr *ProjectRepository) Restore(bs BackupService, name string, repo string) (err error) {
	folder := fr.Path(name)
	if _, err = os.Stat(folder); !os.IsNotExist(err) {
		if !Confirm("the project %s already exists, are you sure you want to restore from a backup?", name) {
			return nil
		}
	}
	return bs.Backup(folder, repo)
}
