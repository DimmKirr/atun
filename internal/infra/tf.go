/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package infra

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/automationd/atun/internal/config"
	"github.com/automationd/atun/internal/logger"
)

const (
	terraformBinaryName = "terraform"
	terraformBaseURL    = "https://releases.hashicorp.com/terraform"
)

// GetTerraformPath returns the path to the Terraform binary
func GetTerraformPath() (string, error) {
	// First check if terraform is in the tunnel directory
	tunnelDir := config.App.Config.TunnelDir
	terraformPath := filepath.Join(tunnelDir, terraformBinaryName)

	// Check if the binary exists
	if _, err := os.Stat(terraformPath); err == nil {
		return terraformPath, nil
	}

	// If not in tunnel directory, check if it's in PATH
	path, err := exec.LookPath(terraformBinaryName)
	if err == nil {
		return path, nil
	}

	return "", fmt.Errorf("terraform not found in %s or PATH", tunnelDir)
}

// getLatestVersion fetches the latest Terraform version from HashiCorp's releases
func getLatestVersion() (string, error) {
	resp, err := http.Get(terraformBaseURL)
	if err != nil {
		return "", fmt.Errorf("failed to fetch terraform releases: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse the content to find the latest stable version
	content := string(body)
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		// Look for lines containing terraform version numbers
		if strings.Contains(line, "terraform_") {
			// Skip alpha, beta, and rc versions
			if strings.Contains(line, "alpha") || strings.Contains(line, "beta") || strings.Contains(line, "rc") {
				continue
			}

			// Extract version number
			parts := strings.Split(line, "terraform_")
			if len(parts) >= 2 {
				// Get everything before the first HTML tag or slash
				version := parts[1]
				if idx := strings.IndexAny(version, "</"); idx != -1 {
					version = version[:idx]
				}
				version = strings.TrimSpace(version)
				return version, nil
			}
		}
	}

	return "", fmt.Errorf("no stable version found in response")
}

// InstallTerraform installs or updates Terraform to the specified version
func InstallTerraform(version string) error {
	// If version is "latest", get the latest version
	if version == "latest" {
		var err error
		version, err = getLatestVersion()
		if err != nil {
			return fmt.Errorf("failed to get latest version: %w", err)
		}
		logger.Debug("Latest Terraform", "version", version)
	}

	// Determine the correct download URL based on OS and architecture
	osName := runtime.GOOS
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "amd64"
	} else if arch == "arm64" {
		arch = "arm64"
	} else {
		return fmt.Errorf("unsupported architecture: %s", arch)
	}

	downloadURL := fmt.Sprintf("%s/%s/terraform_%s_%s_%s.zip", terraformBaseURL, version, version, osName, arch)
	logger.Info("Downloading Terraform", "version", version, "url", downloadURL)

	// Create a temporary file for the download
	tmpFile, err := os.CreateTemp("", "terraform-*.zip")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	// Download the file
	resp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("failed to download terraform: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download terraform: status code %d", resp.StatusCode)
	}

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		return fmt.Errorf("failed to save downloaded file: %w", err)
	}

	// Extract the zip file
	tunnelDir := config.App.Config.TunnelDir
	terraformPath := filepath.Join(tunnelDir, terraformBinaryName)

	// Create the tunnel directory if it doesn't exist
	if err := os.MkdirAll(tunnelDir, 0755); err != nil {
		return fmt.Errorf("failed to create tunnel directory: %w", err)
	}

	// Extract the binary from the zip file
	cmd := exec.Command("unzip", "-o", tmpFile.Name(), terraformBinaryName, "-d", tunnelDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to extract terraform binary: %w", err)
	}

	// Make the binary executable
	if err := os.Chmod(terraformPath, 0755); err != nil {
		return fmt.Errorf("failed to make terraform binary executable: %w", err)
	}

	logger.Info("Successfully installed Terraform", "version", version, "path", terraformPath)
	return nil
}

// CheckTerraformVersion checks if the installed Terraform version matches the required version
func CheckTerraformVersion() error {
	terraformPath, err := GetTerraformPath()
	if err != nil {
		return err
	}

	// Get current version
	cmd := exec.Command(terraformPath, "version")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get terraform version: %w", err)
	}

	// Extract version from output
	versionOutput := string(output)
	versionLine := strings.Split(versionOutput, "\n")[0]
	installedVersion := strings.TrimSpace(strings.TrimPrefix(versionLine, "Terraform v"))
	requiredVersion := config.App.Config.TerraformVersion

	// If required version is "latest", get it
	if requiredVersion == "latest" {
		requiredVersion, err = getLatestVersion()
		if err != nil {
			return fmt.Errorf("failed to get latest version: %w", err)
		}
	}

	// Check if versions match
	if installedVersion != requiredVersion {
		return fmt.Errorf("terraform version mismatch: installed %s, required %s", installedVersion, requiredVersion)
	}

	return nil
}
