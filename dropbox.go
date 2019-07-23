package proj

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	dropbox "github.com/dropbox/dropbox-sdk-go-unofficial/dropbox"
	files "github.com/dropbox/dropbox-sdk-go-unofficial/dropbox/files"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	// "github.com/tj/go-dropbox"
)

type Dropbox struct {
	client        files.Client
	dropboxFolder string
}

func (db *Dropbox) WalkDiffs(local, remote string, skip SkipCallback, cb WalkDiffsCallback) error {
	folders, err := db.client.ListFolder(&files.ListFolderArg{
		Path:             remote,
		IncludeMediaInfo: false,
		Recursive:        true,
		IncludeDeleted:   false})

	var entList []files.IsMetadata
	if err != nil {
		if strings.HasPrefix(err.Error(), "path/not_found/") {
			entList = make([]files.IsMetadata, 0)
			_, err = db.client.CreateFolderV2(files.NewCreateFolderArg(remote))
		}

		if err != nil {
			return err
		}
	} else {
		entList = folders.Entries
	}

	remoteFiles := make(map[string]*files.FileMetadata, len(entList))

	for _, ent := range entList {

		switch ent.(type) {
		case *files.FileMetadata:
			v := ent.(*files.FileMetadata)
			strippedFile, err := filepath.Rel(remote, v.PathDisplay)
			if err != nil {
				return errors.WithMessage(err, "failed to make remote filepath relative")
			}

			remoteFiles[strippedFile] = v
		default:
			// skip everything but normal files
		}
	}

	err = filepath.Walk(local, func(file string, info os.FileInfo, walkErr error) (err error) {
		if walkErr != nil {
			return walkErr
		}

		if info.IsDir() {
			return nil
		}

		strippedFile, err := filepath.Rel(local, file)
		if err != nil {
			return errors.WithMessage(err, "failed to make local filepath relative")
		}

		if skip != nil && skip(strippedFile, info) {
			logrus.Debugf("Skipping %q", strippedFile)
			return nil
		}

		logrus.Debugf("Comparing %q", strippedFile)

		if metadata, exists := remoteFiles[strippedFile]; !exists {
			err = cb(strippedFile, DiffResultOnlyExistsLocal)
		} else {
			if metadata.Size != uint64(info.Size()) {
				err = cb(strippedFile, DiffResultMismatch)
			} else {
				hash, err := db.HashLocal(file)
				if err != nil {
					return err
				}

				if metadata.ContentHash != hash {
					err = cb(strippedFile, DiffResultMismatch)

				}
			}
			if err != nil {
				return err
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

func (db *Dropbox) Delete(f string) (err error) {
	_, err = db.client.DeleteV2(files.NewDeleteArg(f))
	return
}

const chunkSize int64 = 1 << 24

func (db *Dropbox) Upload(local, remote string) (err error) {

	f, err := os.Open(local)
	if err != nil {
		return err
	}

	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	size := info.Size()

	commitInfo := files.NewCommitInfo(remote)
	commitInfo.Mode.Tag = "overwrite"
	commitInfo.ClientModified = time.Now().UTC().Round(time.Second)

	if size > chunkSize {
		res, err := db.client.UploadSessionStart(files.NewUploadSessionStartArg(),
			&io.LimitedReader{R: f, N: chunkSize})
		if err != nil {
			return err
		}

		written := chunkSize

		for (size - written) > chunkSize {
			cursor := files.NewUploadSessionCursor(res.SessionId, uint64(written))
			args := files.NewUploadSessionAppendArg(cursor)

			err = db.client.UploadSessionAppendV2(args, &io.LimitedReader{R: f, N: chunkSize})
			if err != nil {
				return err
			}
			written += chunkSize
		}

		cursor := files.NewUploadSessionCursor(res.SessionId, uint64(written))
		args := files.NewUploadSessionFinishArg(cursor, commitInfo)

		if _, err = db.client.UploadSessionFinish(args, f); err != nil {
			return err
		}
	} else {
		if _, err = db.client.Upload(commitInfo, f); err != nil {
			return err
		}
	}

	return
}

const hashBlockSize = 4 * 1024 * 1024

func ContentHash(r io.Reader) (string, error) {
	buf := make([]byte, hashBlockSize)
	resultHash := sha256.New()
	n, err := r.Read(buf)
	if err != nil && err != io.EOF {
		return "", err
	}
	if n > 0 {
		bufHash := sha256.Sum256(buf[:n])
		resultHash.Write(bufHash[:])
	}
	for n == hashBlockSize && err == nil {
		n, err = r.Read(buf)
		if err != nil && err != io.EOF {
			return "", err
		}
		if n > 0 {
			bufHash := sha256.Sum256(buf[:n])
			resultHash.Write(bufHash[:])
		}
	}
	return fmt.Sprintf("%x", resultHash.Sum(nil)), nil
}

func (db *Dropbox) HashRemote(name string) (hash string, err error) {
	metadata, err := db.client.GetMetadata(
		files.NewGetMetadataArg(name))

	if err != nil {
		return
	}

	return metadata.(*files.FileMetadata).ContentHash, err
}

func (db *Dropbox) HashLocal(file string) (hash string, err error) {
	f, err := os.Open(file)
	if err != nil {
		return
	}

	defer f.Close()

	return ContentHash(f)
}

func (db *Dropbox) Download(local, remote string) (err error) {
	_, result, err := db.client.Download(files.NewDownloadArg(remote))

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

	_, err = io.Copy(f, result)
	return
}

func NewDropbox(token string) *Dropbox {
	return &Dropbox{
		client: files.New(dropbox.Config{Token: token}),
	}
}
