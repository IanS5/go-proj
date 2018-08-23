package proj

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path"

	"github.com/otiai10/copy"
)

var ErrNoResticRepos = errors.New("No restic repos added")
var ErrResticNotFound = errors.New("Restic executable not found")

type BackupService interface {
	Backup(folder string, repositories ...string) error
	Restore(folder, repository string) error
}


type Restic struct{}

// Backup a given resource using restic
func (r Restic) Backup(folder string, repos ...string) (err error) {
	if len(repos) == 0 {
		return ErrNoResticRepos
	}

	restic, err := exec.LookPath("restic")
	if err != nil {
		if os.IsNotExist(err) {
			return ErrResticNotFound
		}
		return err
	}

	for _, r := range repos {
		cmd := exec.Command(restic, "--repo", r, "backup", folder)
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout
		cmd.Stdin = os.Stdin

		err = cmd.Run()
		if err != nil {
			return err
		}

	}
	return
}

func (r Restic) Restore(folder string, repository string) (err error) {
	restic, err := exec.LookPath("restic")
	if err != nil {
		if os.IsNotExist(err) {
			return ErrResticNotFound
		}
		return err
	}

	hashed := sha256.Sum256([]byte(folder))
	tmpdir := path.Join(os.TempDir(), "proj-restic-mount_"+hex.EncodeToString(hashed[:]))
	os.RemoveAll(tmpdir)

	cmd := exec.Command(restic,
		"--repo", repository,
		"restore", "latest",
		"--target", tmpdir,
		"--path", folder)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	err = cmd.Run()
	if err != nil {
		return err
	}

	err = os.Rename(path.Join(tmpdir, folder), folder)
	if err != nil {
		err = copy.Copy(path.Join(tmpdir, folder), folder)
	}
	os.RemoveAll(tmpdir)

	return

}
