package proj

import (
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
	dropbox "github.com/tj/go-dropbox"
)

type Dropbox struct {
	client        *dropbox.Client
	dropboxFolder string
}

func (db *Dropbox) WalkDiffs(local, remote string, cb WalkDiffsCallback) error {
	folders, err := db.client.Files.ListFolder(&dropbox.ListFolderInput{
		Path:             remote,
		Recursive:        true,
		IncludeMediaInfo: false,
		IncludeDeleted:   false,
	})

	if err != nil {

		switch err.(type) {
		case *dropbox.Error:
			// 409 (Not found) is ok
			if err.(*dropbox.Error).StatusCode != 409 {
				return err
			}
		default:
			return err
		}

		folders = &dropbox.ListFolderOutput{
			Entries: []*dropbox.Metadata{},
		}
	}

	remoteFiles := make(map[string]*dropbox.Metadata, len(folders.Entries))

	for _, ent := range folders.Entries {
		if ent.Tag != "folder" {
			remoteFiles[strings.Replace(ent.PathDisplay, remote, "", 1)] = ent
		}
	}

	err = filepath.Walk(local, func(file string, info os.FileInfo, walkErr error) (err error) {
		if walkErr != nil {
			return walkErr
		}

		if info.IsDir() {
			return nil
		}

		strippedFile := strings.Replace(file, local, "", 1)
		if len(strippedFile) > 0 && strippedFile[0] == '/' || strippedFile[0] == '\\' {
			strippedFile = strippedFile[1:]
		}
		logrus.Debugf("Comparing %q", strippedFile)

		if metadata, exists := remoteFiles[strippedFile]; !exists {
			err = cb(strippedFile, DiffResultOnlyExistsLocal)
		} else {
			hash, err := db.HashLocal(file)
			if err != nil {
				return err
			}

			if metadata.ContentHash != hash {
				err = cb(strippedFile, DiffResultMismatch)
				if err != nil {
					return err
				}
			}
			delete(remoteFiles, strippedFile)
		}
		return
	})

	if err != nil {
		return err
	}

	for remotePath := range remoteFiles {
		err = cb(remotePath, DiffResultOnlyExistsRemote)
		if err != nil {
			return err
		}
	}

	return err
}

func (db *Dropbox) Delete(file string) (err error) {
	_, err = db.client.Files.Delete(&dropbox.DeleteInput{
		Path: file,
	})
	return
}

func (db *Dropbox) Upload(local, remote string) (err error) {
	f, err := os.Open(local)
	if err != nil {
		return err
	}

	defer f.Close()

	_, err = db.client.Files.Upload(&dropbox.UploadInput{
		Path:       remote,
		Mode:       dropbox.WriteModeOverwrite,
		AutoRename: false,
		Mute:       true,
		Reader:     f,
	})

	if err != nil {
		return
	}

	return
}

func (db *Dropbox) HashRemote(name string) (hash string, err error) {
	metadata, err := db.client.Files.GetMetadata(
		&dropbox.GetMetadataInput{
			Path:             name,
			IncludeMediaInfo: false,
		})

	if err != nil {
		return
	}

	return metadata.ContentHash, err
}

func (db *Dropbox) HashLocal(file string) (hash string, err error) {
	f, err := os.Open(file)
	if err != nil {
		return
	}

	defer f.Close()

	return dropbox.ContentHash(f)
}

func (db *Dropbox) Download(local, remote string) (err error) {
	result, err := db.client.Files.Download(&dropbox.DownloadInput{
		Path: remote,
	})

	if err != nil {
		return
	}

	err = os.MkdirAll(path.Dir(local), 0775)
	if err != nil {
		return
	}

	f, err := os.Create(local)
	if err != nil {
		return
	}

	_, err = io.Copy(f, result.Body)
	return
}

func NewDropbox(token string) *Dropbox {
	return &Dropbox{
		client: dropbox.New(dropbox.NewConfig(token)),
	}
}
