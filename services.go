package main

import (
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"

	dropbox "github.com/tj/go-dropbox"
)

type DiffResult uint8

const (
	DiffResultSame = DiffResult(iota)
	DiffResultDeleted
	DiffResultCreated
	DiffResultUpdated
)

type StorageService interface {
}

type Dropbox struct {
	client *dropbox.Client
	name   string
	rc     Resource
}

func (dr *DiffResult) String() string {
	switch *dr {
	case DiffResultSame:
		return "SAME"
	case DiffResultCreated:
		return "CREATED"
	case DiffResultDeleted:
		return "DELETED"
	case DiffResultUpdated:
		return "UPDATED"
	}
	return "UNKNOWN"
}

func (db *Dropbox) walkDiffs(cb func(string, DiffResult) error) error {
	folders, err := db.client.Files.ListFolder(&dropbox.ListFolderInput{
		Path:             db.GetRemotePath(""),
		Recursive:        true,
		IncludeMediaInfo: false,
		IncludeDeleted:   false,
	})

	if err != nil {

		switch err.(type) {
		case *dropbox.Error:
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

	localPrefix := db.GetLocalPath("") + "/"
	remotePrefix := db.GetRemotePath("")

	for _, ent := range folders.Entries {
		if ent.Tag != "folder" {
			remoteFiles[strings.Replace(ent.Name, remotePrefix, "", 1)] = ent
		}
	}

	err = filepath.Walk(db.GetLocalPath(""), func(file string, info os.FileInfo, walkErr error) (err error) {
		if walkErr != nil {
			return walkErr
		}

		if info.IsDir() {
			return nil
		}

		strippedFile := strings.Replace(file, localPrefix, "", 1)

		if metadata, exists := remoteFiles[strippedFile]; !exists {
			err = cb(strippedFile, DiffResultCreated)
		} else {
			hash, err := db.HashLocal(strippedFile)
			if err != nil {
				return err
			}

			if metadata.ContentHash != hash {
				err = cb(strippedFile, DiffResultUpdated)
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
		err = cb(remotePath, DiffResultDeleted)
		if err != nil {
			return err
		}
	}

	return err
}

func (db *Dropbox) dropboxFolder() string {
	return path.Join("/"+db.rc.String(), db.name)
}

func (db *Dropbox) GetRemotePath(file string) string {
	return path.Join(db.dropboxFolder(), file)
}

func (db *Dropbox) GetLocalPath(file string) string {
	return path.Join(db.rc.Directory(), db.name, file)
}

func (db *Dropbox) Delete(file string) (err error) {
	_, err = db.client.Files.Delete(&dropbox.DeleteInput{
		Path: db.GetRemotePath(file),
	})
	return
}

func (db *Dropbox) Upload(file string) (err error) {
	f, err := os.Open(db.GetLocalPath(file))
	if err != nil {
		return err
	}

	defer f.Close()

	_, err = db.client.Files.Upload(&dropbox.UploadInput{
		Path:       db.GetRemotePath(file),
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

func (db *Dropbox) Sync() (err error) {
	_, err = db.rc.Instance(db.name)
	if err != nil {
		return err
	}

	return db.walkDiffs(func(file string, diff DiffResult) (err error) {
		log.Printf("(%s) %s", diff.String(), file)

		switch diff {
		case DiffResultCreated, DiffResultUpdated:
			err = db.Upload(file)
		case DiffResultDeleted:
			err = db.Delete(file)
		case DiffResultSame:
		default:
		}
		return
	})
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
	const HashChunkSize = 4 * 1024 * 1024

	f, err := os.Open(path.Join(db.rc.Directory(), db.name, file))
	if err != nil {
		return
	}

	defer f.Close()

	return dropbox.ContentHash(f)
}
func (db *Dropbox) Download(file string) (err error) {
	result, err := db.client.Files.Download(&dropbox.DownloadInput{
		Path: db.GetRemotePath(file),
	})

	if err != nil {
		return
	}

	f, err := os.Create(db.GetLocalPath(file))
	if err != nil {
		return
	}

	_, err = io.Copy(f, result.Body)
	return
}

func (db *Dropbox) Pull() (err error) {
	_, err = db.rc.Instance(db.name)
	if err != nil {
		err = db.rc.Create(db.name, "")
		if err != nil {
			return
		}

		_, err = db.rc.Instance(db.name)
		if err != nil {
			return
		}
	}

	return db.walkDiffs(func(file string, diff DiffResult) (err error) {
		switch diff {
		case DiffResultSame:
		case DiffResultDeleted, DiffResultUpdated:
			log.Printf("(DOWNLOAD) %s", file)
			db.Download(file)
		case DiffResultCreated:
			log.Printf("(DELETE) %s", file)
			err = os.Remove(db.GetLocalPath(file))
		}
		return
	})
}

func NewDropboxClient(rc Resource, inst string, token string) *Dropbox {
	return &Dropbox{
		client: dropbox.New(dropbox.NewConfig(token)),
		name:   inst,
		rc:     rc,
	}
}
