package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
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
			remoteFiles[strings.Replace(ent.Name, remote, "", 1)] = ent
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
		log.Debugf("Comparing %q", strippedFile)

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

func (db *Dropbox) Download(remote, local string) (err error) {
	result, err := db.client.Files.Download(&dropbox.DownloadInput{
		Path: remote,
	})

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

func DropboxInvalidateToken(token string) (err error) {
	req, err := http.NewRequest("POST", "https://api.dropboxapi.com/2/auth/token/revoke", nil)
	if err != nil {
		return
	}
	req.Header.Add("Authorization", "Bearer "+token)
	_, err = http.DefaultClient.Do(req)
	return
}

func InteractiveDropboxLogin() (token string, err error) {
	callbackURL := "localhost:8314"
	authURL :=
		fmt.Sprintf(
			"https://www.dropbox.com/oauth2/authorize/?response_type=%s&client_id=%s&redirect_uri=%s",
			"code",
			"av3dt43wyk1hhz7",
			url.QueryEscape("http://"+callbackURL),
		)

	fmt.Printf("Please navigate to \"%s\" in your browser\n", authURL)
	server := &http.Server{Addr: callbackURL}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	code := ""
	var handlerErr error
	server.Handler = http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			handlerErr = r.ParseForm()
			if handlerErr != nil {
				fmt.Fprint(w, "Bad URL")
				cancel()
				return
			}

			code = r.FormValue("code")
			if code != "" {
				fmt.Fprint(w, "Yay! Everything's all good. You can close this tab and navigate back to your terminal.")
			} else {
				fmt.Fprint(w, "Missing token")
			}
			cancel()
		},
	)

	go func() {
		err = server.ListenAndServe()
		if handlerErr != nil {
			err = handlerErr
		}
	}()

	select {
	case <-ctx.Done():
		server.Shutdown(ctx)
		if err != nil {
			return
		}
	}

	if code == "" {
		return "", errors.New("Request was missing code URL parameter")
	}

	var response struct {
		Token     string `json:"access_token"`
		TokenType string `json:"token_type"`
		AccountID string `json:"account_id"`
		UserID    string `json:"uid"`
	}

	log.Info("Fetching account token...")

	resp, err := http.PostForm("https://api.dropboxapi.com/oauth2/token", url.Values{
		"code":          []string{code},
		"grant_type":    []string{"authorization_code"},
		"client_id":     []string{"av3dt43wyk1hhz7"},
		"client_secret": []string{"***REMOVED***"},
		"redirect_uri":  []string{"http://" + callbackURL},
	})

	if err != nil {
		return
	}

	if resp.StatusCode != 200 {
		body, _ := ioutil.ReadAll(resp.Body)

		log.
			WithField("Status", resp.StatusCode).
			WithField("Body", string(body)).
			Fatal("Bad Status")
	}
	log.Info("Done!")

	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		log.Fatal(err)
	}

	return response.Token, err
}
