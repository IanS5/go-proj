package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/otiai10/copy"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

// A Resource is a type of folder that can have an [Operation] applied to it
// Every resource gets its own directory.
type Resource uint8

const (
	// Template is a folder that will be copied into another location to create a project
	Template = Resource(iota)

	// Project is a single working directory for the user
	Project
)

var (
	ErrNoResticRepos       = errors.New("No restic repos found, please specify $PROJ_RESTIC_REPOS")
	ErrResticNotFound      = errors.New("Could not find restic in $PATH")
	ErrOptionOutOfRange    = errors.New("Invalid response, please choose one of the provided options")
	ErrMissingDropboxToken = errors.New("No dropbox token found, please set $PROJ_DROPBOX_TOKEN")
)

// Flags is a structure containing all the CLI flags and Args
type Flags struct {
	Target Resource

	ResourceName string
	TemplateName string

	Create  bool
	List    bool
	Remove  bool
	Backup  bool
	Restore bool
	Visit   bool
	Sync    bool
	Pull    bool
}

// getenvOr gets an environment variable or returns a default value if the variable was not set
func getenvOr(env string, dflt string) string {
	v := os.Getenv(env)
	if v == "" {
		return dflt
	}
	return v

}

// clearScreen clears the screen
func clearScreen() {
	// TODO: support windows CMD.EXE
	print("\033[2J\033[H")
}

// imax is an integer max function
func imax(x, y int) int {
	if x > y {
		return x
	}

	return y
}

// confirm some action, ask the user to input y/n.
func confirm(question string, args ...interface{}) bool {
	fmt.Printf("%s [Y/n] ", fmt.Sprintf(question, args...))
	response := ""
	fmt.Scanln(&response)
	switch strings.Trim(strings.ToLower(response), "\t\r\n\v ") {
	case "y", "ye", "yes":
		return true
	default:
		return false
	}
}

// Ask the user to pick one of several options, return the one they picked
func choose(question string, options []string, args ...interface{}) (choice string, err error) {
	optionStr := strings.Builder{}
	optionStr.Grow(12)

	for i, s := range options {
		fmt.Printf(" %d) %s\n", i+1, s)
		if i < 3 {
			if i != 0 {
				optionStr.WriteString("/")
			}
			optionStr.WriteString(strconv.Itoa(i + 1))
		}
	}
	if len(options) > 3 {
		optionStr.WriteString("...")
	}

	fmt.Printf("%s [%s] ", fmt.Sprintf(question, args...), optionStr.String())
	response := ""
	fmt.Scanln(&response)
	option, err := strconv.Atoi(strings.Trim(response, "\t\r\n\v "))
	if err != nil {
		return
	}

	if option > len(options) || option < 1 {
		return "", ErrOptionOutOfRange
	}

	return options[option-1], err
}

// ResticRepos lissts the user's restic repositories
func ResticRepos() []string {
	r := os.Getenv("PROJ_RESTIC_REPOS")
	if r != "" {
		return strings.Split(os.Getenv("PROJ_RESTIC_REPOS"), string(os.PathListSeparator))
	} else {
		return []string{}
	}
}

// Root gets proj's root directory
func Root() string {
	return getenvOr("PROJ_ROOT_DIR", path.Join(os.Getenv("HOME"), ".proj"))
}

func HistDir() string {
	return getenvOr("PROJ_HIST_DIR", path.Join(Root(), ".hist"))
}

// Directory gets the resource's root directory
func (rc Resource) Directory() string {
	switch rc {
	case Project:
		return getenvOr("PROJ_PROJECT_DIR", path.Join(Root(), "projects"))
	case Template:
		return getenvOr("PROJ_TEMPLATE_DIR", path.Join(Root(), "templates"))
	}

	log.Fatal("Invalid resource")
	return ""
}

func (rc Resource) Sync(name string) (err error) {
	tok := os.Getenv("PROJ_DROPBOX_TOKEN")
	if tok == "" {
		return ErrMissingDropboxToken
	}

	db := NewDropboxClient(rc, name, tok)
	return db.Sync()
}

func (rc Resource) Instance(name string) (p string, err error) {
	p = path.Join(rc.Directory(), name)
	_, err = os.Stat(p)
	return
}

func (rc Resource) Pull(name string) (err error) {
	tok := os.Getenv("PROJ_DROPBOX_TOKEN")
	if tok == "" {
		return ErrMissingDropboxToken
	}

	db := NewDropboxClient(rc, name, tok)
	return db.Pull()
}

func (rc Resource) Create(name string, template string) (err error) {
	p := path.Join(rc.Directory(), name)

	if template != "" {
		templates := Template
		tmpl, err := templates.Instance(template)
		if err != nil {
			return err
		}

		err = copy.Copy(tmpl, p)
	} else {
		err = os.Mkdir(p, 0755)
	}

	if err != nil {
		return err
	}

	initFile := path.Join(p, "PROJINIT")
	if _, maybeNotExists := os.Stat(initFile); !os.IsNotExist(maybeNotExists) {
		// TODO: handle the bang instead
		cmd := exec.Command("bash", initFile)
		cmd.Dir = p
		cmd.Env = append(cmd.Env, "PROJECT="+name, "TEMPLATE="+template)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err = cmd.Run()
		if err != nil {
			return
		}

		err = os.Remove(initFile)
	}
	return
}

// Remove the resource instance
func (rc Resource) Remove(name string) (err error) {
	p, err := rc.Instance(name)
	if err != nil {
		return
	}

	if !confirm("Are you sure you want to remove %s?", name) {
		return
	}

	err = os.RemoveAll(p)
	return
}

// List searchs through the resource instance(s) using a regex pattern
func (rc Resource) List(rules ...string) (err error) {
	compiledRules := make([]*regexp.Regexp, 0, len(rules))
	for _, rule := range rules {
		compiled, err := regexp.Compile(rule)
		if err != nil {
			return err
		}
		compiledRules = append(compiledRules, compiled)
	}
	entries, err := ioutil.ReadDir(rc.Directory())
	if err != nil {
		return
	}

	for _, f := range entries {
		canWrite := true
		for _, r := range compiledRules {
			if !r.MatchString(f.Name()) {
				canWrite = false
				break
			}
		}

		if canWrite {
			fmt.Println(f.Name())
		}
	}
	return
}

func (rc Resource) GetInstances() (insts []string, err error) {
	entries, err := ioutil.ReadDir(rc.Directory())
	insts = make([]string, len(entries))
	if err != nil {
		return
	}

	for _, f := range entries {
		insts = append(insts, f.Name())
	}
	return
}

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

func (rc Resource) String() string {
	switch rc {
	case Template:
		return "Template"
	case Project:
		return "Project"
	default:
		return "?"
	}
}

func (rc Resource) Id(name string) string {
	out := strings.Builder{}
	encoder := base64.NewEncoder(base64.StdEncoding, &out)
	encoder.Write([]byte(rc.String()))
	encoder.Write([]byte("##"))
	encoder.Write([]byte(name))
	return out.String()
}

func (rc Resource) HistFile(name string) string {
	return path.Join(HistDir(), rc.Id(name))
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

// Visit a resource instance
func (rc Resource) Visit(name string) (err error) {
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

	instancePath, err := rc.Instance(name)
	if err != nil {
		return err
	}

	baseEnvVar := ""
	nameEnvVar := ""

	switch rc {
	case Project:
		baseEnvVar = "PROJ_CURRENT_PROJECT_BASE"
		nameEnvVar = "PROJ_CURRENT_PROJECT_NAME"
	case Template:
		baseEnvVar = "PROJ_CURRENT_TEMPLATE_BASE"
		nameEnvVar = "PROJ_CURRENT_TEMPLATE_NAME"
	}

	env := modEnviron(map[string]string{
		baseEnvVar:     instancePath,
		nameEnvVar:     name,
		"HISTFILE":     rc.HistFile(name),
		"fish_history": rc.Id(name),
	})

	err = os.Chdir(instancePath)
	if err != nil {
		return err
	}

	clearScreen()
	return syscall.Exec(shell, []string{invokedExe}, env)
}

func main() {
	app := kingpin.New("proj", "A stupid simple project manager")
	app.Version("0.1.0")
	app.Author("Ian Shehadeh <IanShehadeh2020@gmail.com>")

	flags := Flags{}
	projectFlag := false
	templateFlag := false

	app.Arg("RESOURCE", "The resource which will be operated on").HintAction(func() []string {
		options, _ := flags.Target.GetInstances()
		return options
	}).StringVar(&flags.ResourceName)

	app.Arg("TEMPLATE", "When creating a project it will be based off of this template").HintAction(func() []string {
		options, _ := Template.GetInstances()
		return options
	}).StringVar(&flags.TemplateName)

	app.Flag("create", "Create a new instance of a resource").Short('c').BoolVar(&flags.Create)
	app.Flag("list", "list all instances of a resource").Short('l').BoolVar(&flags.List)
	app.Flag("remove", "Remove an instance of a resource").Short('r').BoolVar(&flags.Remove)
	app.Flag("backup", "Backup a resource instance").Short('b').BoolVar(&flags.Backup)
	app.Flag("restore", "Restore from a backup").Short('e').BoolVar(&flags.Restore)
	app.Flag("visit", "Run a new instance of the current shell in this resource's directory").Short('v').BoolVar(&flags.Visit)
	app.Flag("sync", "Sync the project with its origin").Short('s').BoolVar(&flags.Sync)
	app.Flag("pull", "Pull the project from its origin").Short('p').BoolVar(&flags.Pull)

	app.Flag("project", "Operate on a project").Short('P').BoolVar(&projectFlag)
	app.Flag("template", "Operate on a template").Short('T').BoolVar(&templateFlag)

	app.Parse(os.Args[1:])

	if projectFlag && templateFlag {
		app.Fatalf("--project (-P) and --template (-T) are exclusive")
	} else if projectFlag {
		flags.Target = Project
	} else if templateFlag {
		flags.Target = Template
	} else {
		app.Fatalf("Please specify either --project (-P) or --template (-T)")
	}

	var err error

	if flags.Remove {
		err = flags.Target.Remove(flags.ResourceName)
	}

	if flags.Create {
		err = flags.Target.Create(flags.ResourceName, flags.TemplateName)
	}

	if flags.Pull {
		err = flags.Target.Pull(flags.ResourceName)
	}

	if flags.Restore {
		err = flags.Target.Restore(flags.ResourceName)
	}

	if flags.Visit {
		err = flags.Target.Visit(flags.ResourceName)
	}

	if flags.Backup {
		err = flags.Target.Backup(flags.ResourceName)
	}

	if flags.List {
		if flags.ResourceName != "" {
			err = flags.Target.List(flags.ResourceName)
		} else {
			err = flags.Target.List()
		}
	}

	if flags.Sync {
		err = flags.Target.Sync(flags.ResourceName)
	}

	if err != nil {
		log.Fatal(err)
	}
}
