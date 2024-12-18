/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package version

import (
	"encoding/json"
	"fmt"
	"github.com/Masterminds/semver"
	"github.com/automationd/atun/internal/logger"
	"github.com/pterm/pterm"
	"net/http"
	"runtime"
	"runtime/debug"
	"time"
)

var (
	GitCommit string
	Version   = "0.0.0"
)

func GetVersion() (ret string) {
	if b, ok := debug.ReadBuildInfo(); ok && len(b.Main.Version) > 0 {
		ret = b.Main.Version
	} else {
		ret = "unknown"
	}
	return
}

func FullVersionNumber() string {
	var v = ""

	v = v + fmt.Sprintf("%s", Version)

	if Version == "dev" || Version == "0.0.0" {
		v = v + fmt.Sprintf(" dev %s", time.Now().Format("2006-01-02T15:04:05"))
	}

	if GitCommit != "" {
		v = v + fmt.Sprintf(" (%s)", GitCommit)
	}

	return v
}

func CheckLatestRelease() {
	_, err := semver.NewVersion(Version)
	if err != nil {
		return
	}

	resp, err := http.Get("https://api.github.com/repos/automationd/atun/releases/latest")
	if err != nil {
		logger.Error("Failed to check for the latest version", "error", err)
	}

	var gr gitResponse

	// Handle the case when Github API won't respond (like rate limiting)
	if resp.StatusCode != 200 {
		logger.Debug("Failed to check for the latest version", "error", fmt.Errorf("status code: %d", resp.StatusCode))
		gr.Version = "unknown"
	} else {
		if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil {
			logger.Fatal("Failed to check for the latest version", "error", fmt.Errorf("status code: %d"))
		}
	}

	if Version == "0.0.0" || Version == "development" || Version == "unknown" {
		err = ShowUpgradeCommand(true)
		if err != nil {
			logger.Fatal("Failed to show upgrade command", "error", err)
		}
	} else {
		var versionChangeAction = "upgrading"
		if Version > gr.Version {
			versionChangeAction = "downgrading"
		}

		if Version != gr.Version {
			pterm.Warning.Printfln("The newest stable version is %s, but your version is %s. Consider %s.", gr.Version, Version, versionChangeAction)
			ShowUpgradeCommand(false)
		}
	}

}

type gitResponse struct {
	Version string `json:"tag_name"`
}

func ShowUpgradeCommand(isDev bool) error {
	switch goos := runtime.GOOS; goos {
	case "darwin":
		if isDev {
			pterm.Debug.Printfln("To install latest:\n`brew update && brew fetch --force atun && brew reinstall --build-from-source atun`")
		} else {
			pterm.Info.Println("Use the command to update: `brew upgrade atun`")
		}
	//case "linux":
	//	distroName, err := requirements.ReadOSRelease("/etc/os-release")
	//	if err != nil {
	//		return err
	//	}
	//	switch distroName["ID"] {
	//	case "ubuntu":
	//		pterm.Warning.Println("Use the command to update:\n\tapt update && apt install ize")
	//	default:
	//		pterm.Warning.Println("See https://github.com/hazelops/ize/blob/main/DOCS.md#installation")
	//	}a
	default:
		pterm.Warning.Println("See https://github.com/automationd/atun/blob/main/README.md#installation")
	}

	return nil
}
