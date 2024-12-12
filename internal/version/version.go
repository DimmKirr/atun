/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package version

import (
	"encoding/json"
	"fmt"
	"github.com/Masterminds/semver"
	"github.com/pterm/pterm"
	"log"
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
		log.Fatalln(err)
	}

	var gr gitResponse

	if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil {
		log.Fatal(err)
	}

	var versionChangeAction = "upgrading"
	if Version > gr.Version {
		versionChangeAction = "downgrading"
	}
	if Version != gr.Version {
		pterm.Warning.Printfln("The newest stable version is %s, but your version is %s. Consider %s.", gr.Version, Version, versionChangeAction)
		ShowUpgradeCommand()
	}
}

type gitResponse struct {
	Version string `json:"tag_name"`
}

func ShowUpgradeCommand() error {
	switch goos := runtime.GOOS; goos {
	case "darwin":
		pterm.Warning.Println("Use the command to update: `brew upgrade atun`")
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
