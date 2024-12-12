/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package constraints

import (
	"fmt"
	"github.com/automationd/atun/internal/logger"
	"github.com/go-ini/ini"
	"github.com/pterm/pterm"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
)

func CheckCommand(command string, subcommand []string) (bool, string) {
	cmd := exec.Command(command, subcommand...)

	// Capture combined stdout and stderr
	out, err := cmd.CombinedOutput()
	output := string(out)

	// Log the output at debug level
	logger.Debug(output)

	// Return success status and output
	if err != nil {
		return false, output
	}
	return true, output
}

const (
	ssmLinuxUrl = "https://s3.amazonaws.com/session-manager-downloads/plugin/latest/%s_%s/session-manager-plugin%s"
	ssmMacOsUrl = "https://s3.amazonaws.com/session-manager-downloads/plugin/latest/mac/sessionmanager-bundle.zip"
)

func downloadSSMAgentPlugin() error {
	switch goos := runtime.GOOS; goos {
	case "darwin":
		client := http.Client{
			CheckRedirect: func(r *http.Request, via []*http.Request) error {
				r.URL.Opaque = r.URL.Path
				return nil
			},
		}

		file, err := os.Create("session-manager-plugin.deb")
		if err != nil {
			log.Fatal(err)
		}

		resp, err := client.Get(ssmMacOsUrl)
		if err != nil {
			log.Fatal(err)
		}

		defer resp.Body.Close()

		_, err = io.Copy(file, resp.Body)
		if err != nil {
			return err
		}

		defer file.Close()
	case "linux":
		distroName, err := GetOSRelease("/etc/os-release")
		if err != nil {
			return err
		}

		arch := ""

		switch runtime.GOARCH {
		case "amd64":
			arch = "64bit"
		case "386":
			arch = "32bit"
		case "arm":
			arch = "arm32"
		case "arm64":
			arch = "arm64"
		}

		client := http.Client{
			CheckRedirect: func(r *http.Request, via []*http.Request) error {
				r.URL.Opaque = r.URL.Path
				return nil
			},
		}

		switch distroName["ID"] {
		case "ubuntu", "debian":
			file, err := os.Create("session-manager-plugin.deb")
			if err != nil {
				log.Fatal(err)
			}

			defer file.Close()

			resp, err := client.Get(fmt.Sprintf(ssmLinuxUrl, "ubuntu", arch, ".deb"))
			if err != nil {
				log.Fatal(err)
			}

			defer resp.Body.Close()

			_, err = io.Copy(file, resp.Body)
			if err != nil {
				return err
			}
		default:
			file, err := os.Create("session-manager-plugin.rpm")
			if err != nil {
				log.Fatal(err)
			}

			defer file.Close()

			resp, err := client.Get(fmt.Sprintf(ssmLinuxUrl, "linux", arch, ".rpm"))
			if err != nil {
				log.Fatal(err)
			}

			defer resp.Body.Close()

			_, err = io.Copy(file, resp.Body)
			if err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unable to install automatically")
	}

	return nil
}

func cleanupSSMAgent() error {
	command := []string{}

	if runtime.GOOS == "darwin" {
		command = []string{"rm", "-f", "sessionmanager-bundle sessionmanager-bundle.zip"}
	} else if runtime.GOOS == "linux" {
		distroName, err := GetOSRelease("/etc/os-release")
		if err != nil {
			return err
		}
		switch distroName["ID"] {
		case "ubuntu", "debian":
			command = []string{"rm", "-rf", "session-manager-plugin.deb"}
		default:
			command = []string{"rm", "-f", "session-manager-plugin.rpm"}
		}
	}

	cmd := exec.Command(command[0], command[1:]...)

	err := cmd.Run()
	if err != nil {
		return err
	}

	return nil
}

func installSSMAgent() error {
	command := []string{}

	if runtime.GOOS == "darwin" {
		command = []string{"sudo", "./sessionmanager-bundle/install", "-i /usr/local/sessionmanagerplugin", "-b", "/usr/local/bin/session-manager-plugin"}
	} else if runtime.GOOS == "linux" {
		command = []string{"sudo", "yum", "install", "-y", "-q", "session-manager-plugin.deb"}

		distroName, err := GetOSRelease("/etc/os-release")
		if err != nil {
			return err
		}
		switch distroName["ID"] {
		case "ubuntu", "debian":
			command = []string{"sudo", "dpkg", "-i", "session-manager-plugin.deb"}
		case "fedora":
			command = []string{"sudo", "dnf", "install", "session-manager-plugin.rpm"}
		case "rhel":
			command = []string{"sudo", "yum", "install", "session-manager-plugin.rpm"}
		}
	} else {
		return fmt.Errorf("automatic installation of SSM Agent for your OS is not supported")
	}

	cmd := exec.Command(command[0], command[1:]...)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}

	pterm.Info.Println(string(out))

	return nil
}

func GetOSRelease(configfile string) (map[string]string, error) {
	cfg, err := ini.Load(configfile)
	if err != nil {
		return nil, err
	}

	ConfigParams := make(map[string]string)
	ConfigParams["ID"] = cfg.Section("").Key("ID").String()

	return ConfigParams, nil
}
