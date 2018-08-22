
// Backup a given resource using restic
func (rc Resource) Backup(name string) (err error) {
	repos := ResticRepos()
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

	dir, err := rc.Instance(name)
	if err != nil {
		return err
	}

	for _, r := range repos {
		cmd := exec.Command(restic, "--repo", r, "backup", dir)
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

// Restore restores a resource instance from the latest backup
func (rc Resource) Restore(name string) (err error) {
	repos := ResticRepos()
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

	dir, err := rc.Instance(name)
	if !os.IsNotExist(err) {
		if !confirm("%s already exists, are you sure you want to restore from a backup?", name) {
			return nil
		}
		err = os.RemoveAll(dir)
		if err != nil {
			return err
		}

	}

	repo, err := choose("Please select a rustic repository", repos)
	if err != nil {
		return err
	}
	tmpdir := path.Join(os.TempDir(), "proj-restic-mount_"+name)
	os.RemoveAll(tmpdir)

	cmd := exec.Command(restic,
		"--repo", repo,
		"restore", "latest",
		"--target", tmpdir,
		"--path", dir)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	err = cmd.Run()
	if err != nil {
		return err
	}

	err = os.Rename(path.Join(tmpdir, dir), dir)
	if err != nil {
		err = copy.Copy(path.Join(tmpdir, dir), dir)
	}
	os.RemoveAll(tmpdir)

	return

}