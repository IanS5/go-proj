package proj

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path"

	"github.com/sirupsen/logrus"
)

var ConfigPath = path.Join(os.Getenv("HOME"), ".proj", "cache.json")

type Config struct {
	Dropbox struct {
		AppKey    string `json:"app-key"`
		AppSecret string `json:"app-secret"`
		Token     string `json:"token"`
	} `json:"dropbox"`

	Restic struct {
		Repositories []string `json:"repositories"`
	} `json:"restic"`

	ProjectRepositories map[string]string `json:"project-repositories"`
	PrimaryRepo         string            `json:"primary-repo"`
}

func LoadConfig() (cfg *Config) {
	cfg = &Config{}
	if data, err := ioutil.ReadFile(ConfigPath); err != nil {
		cfg.ProjectRepositories = make(map[string]string)
		cfg.Restic.Repositories = make([]string, 0)
		return
	} else {
		err = json.Unmarshal(data, cfg)
		if err != nil {
			logrus.
				WithError(err).
				Info("Failed to read config, starting fresh...")
			cfg.ProjectRepositories = make(map[string]string)
			cfg.Restic.Repositories = make([]string, 0)
			cfg.Dropbox.AppKey = ""
			cfg.Dropbox.AppSecret = ""
			cfg.Dropbox.Token = ""
		}
		return
	}
}

func (cfg *Config) Write() {
	f, err := os.Create(ConfigPath)
	if err != nil {
		logrus.
			WithError(err).
			Info("Failed to open config file...")

		if os.IsNotExist(err) {
			dir := path.Dir(ConfigPath)
			logrus.WithField("Dir", dir).Info("Creating the necessary directories")
			os.MkdirAll(dir, 0755)

			logrus.WithField("Dir", dir).Info("Retry...")
			f, err = os.Create(ConfigPath)
			if err == nil {
				logrus.Info("Success")
			} else {
				logrus.WithError(err).Info("Second attempt failed")
				return
			}
		} else {
			logrus.WithError(err).Error("Cannot handle error")
			return
		}
	}

	err = json.NewEncoder(f).Encode(cfg)
	if err != nil {
		logrus.
			WithError(err).
			Error("Failed to write config")
	}
}
