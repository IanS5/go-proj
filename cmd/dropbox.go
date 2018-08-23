package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func dropboxLogin(appKey string, appSecret string) (token string, err error) {
	callbackURL := "localhost:8314"
	authURL :=
		fmt.Sprintf(
			"https://www.dropbox.com/oauth2/authorize/?response_type=%s&client_id=%s&redirect_uri=%s",
			"code",
			appKey,
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

	logrus.Info("Fetching account token...")

	resp, err := http.PostForm("https://api.dropboxapi.com/oauth2/token", url.Values{
		"code":          []string{code},
		"grant_type":    []string{"authorization_code"},
		"client_id":     []string{appKey},
		"client_secret": []string{appSecret},
		"redirect_uri":  []string{"http://" + callbackURL},
	})

	if err != nil {
		return
	}

	if resp.StatusCode != 200 {
		body, _ := ioutil.ReadAll(resp.Body)

		logrus.
			WithField("Status", resp.StatusCode).
			WithField("Body", string(body)).
			Fatal("Bad Status")
	}
	logrus.Info("Done!")

	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		logrus.Fatal(err)
	}

	return response.Token, err
}

var cmdDropbox = &cobra.Command{
	Use:   "dropbox",
	Short: "Manage your the dropbox account associated with proj",
}

var cmdDropboxLogout = &cobra.Command{
	Use:   "logout",
	Short: "Logout of the dropbox account associated with proj",
	Run: func(cmd *cobra.Command, args []string) {
		req, err := http.NewRequest("POST", "https://api.dropboxapi.com/2/auth/token/revoke", nil)
		if err != nil {
			return
		}
		req.Header.Add("Authorization", "Bearer "+config.Dropbox.Token)
		_, err = http.DefaultClient.Do(req)

		if err != nil {
			logrus.WithError(err).Fatal("Logout failed")
		}

		config.Dropbox.Token = ""
		config.Write()
		return
	},
}

var cmdDropboxLogin = &cobra.Command{
	Use:   "login",
	Short: "Login to your dropbox account",
	Run: func(cmd *cobra.Command, args []string) {
		if config.Dropbox.AppKey == "" && config.Dropbox.AppSecret == "" {
			logrus.Fatal("Missing app key and secret, set them with proj dropbox app KEY SECRET")
		} else if config.Dropbox.AppKey == "" {
			logrus.Fatal("Missing app key")
		} else if config.Dropbox.AppSecret == "" {
			logrus.Fatal("Missing app secret")
		}

		token, err := dropboxLogin(config.Dropbox.AppKey, config.Dropbox.AppSecret)

		if err != nil {
			logrus.WithError(err).Fatal("Login failed")
		}

		config.Dropbox.Token = token
		config.Write()
	},
}

var cmdDropboxApp = &cobra.Command{
	Use:   "app KEY SECRET",
	Short: "Add your dropbox application credentials",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		config.Dropbox.AppKey = args[0]
		config.Dropbox.AppSecret = args[1]

		config.Write()
	},
}

func init() {
	cmdDropbox.AddCommand(cmdDropboxLogout, cmdDropboxLogin, cmdDropboxApp)
}
