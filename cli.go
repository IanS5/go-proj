package proj

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

var (
	// ErrOptionOutOfRange is returned by Choose if the user's response is invalid
	ErrOptionOutOfRange = errors.New("Invalid response, please choose one of the provided options")
)

func normalizeResponse(x string) string {
	return strings.Trim(strings.ToLower(x), "\t\r\n\v ")
}

// ClearScreen clears the user's terminal
func ClearScreen() {
	if runtime.GOOS == "windows" {
		exec.Command("cls").Run()
	} else {
		print("\033[2J\033[H")
	}
}

// Confirm an action, ask the user to input y/n.
func Confirm(question string, args ...interface{}) bool {
	fmt.Printf("%s [Y/n] ", fmt.Sprintf(question, args...))
	response := ""
	fmt.Scanln(&response)
	switch normalizeResponse(response) {
	case "y", "ye", "yes":
		return true
	default:
		return false
	}
}

// Choose asks the user to pick one of several options, return the one they picked
func Choose(question string, options []string, args ...interface{}) (choice string, err error) {
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

// GetOpts gets all options in the form of -(.*) or --(.+)
func GetOpts(args ...string) (opts []string) {
	opts = make([]string, 0, len(args))

	for _, a := range args {
		if len(a) >= 2 && a[:2] == "--" {
			if len(a) == 2 {
				break
			} else {
				opts = append(opts, a[2:])
			}
		} else if len(a) >= 1 && a[0] == '-' {
			for _, o := range a[1:] {
				opts = append(opts, string(o))
			}
		} else {
			break
		}
	}

	return
}
