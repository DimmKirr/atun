/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package aws

import (
	"fmt"
	"github.com/automationd/atun/internal/logger"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/service/iam"
	"os"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"gopkg.in/ini.v1"
)

type SessionConfig struct {
	Region                   string
	Profile                  string
	EndpointUrl              string
	SharedCredentialsPath    string
	MFASharedCredentialsPath string
}

func GetSession(sessionConfig *SessionConfig) (*session.Session, error) {

	// Check if the env var is set and if not set it to the default value. (Maybe there is a better way to do this?)
	credFilePath := os.Getenv("AWS_SHARED_CREDENTIALS_FILE")
	if credFilePath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		credFilePath = path.Join(homeDir, ".aws/credentials")
	}

	// Get region from the credentials file if it's not set
	credFile, err := ini.Load(credFilePath)
	if err != nil {
		return nil, err
	}

	profile, err := credFile.GetSection(sessionConfig.Profile)
	if err != nil {
		return nil, err
	}

	if sessionConfig.Region == "" {
		logger.Debug("Region is not set, trying to get it from the credentials file", "profile", sessionConfig.Profile)
		sessionConfig.Region = profile.Key("region").String()
	}

	var config *aws.Config

	if sessionConfig.EndpointUrl != "" {
		config = aws.NewConfig().WithRegion(sessionConfig.Region).WithCredentials(credentials.NewSharedCredentials(credFilePath, sessionConfig.Profile)).WithEndpoint(sessionConfig.EndpointUrl)
		logger.Debug("Session established", "credFilePath", credFilePath, "endpoint", sessionConfig.EndpointUrl)
	} else {
		config = aws.NewConfig().WithRegion(sessionConfig.Region).WithCredentials(credentials.NewSharedCredentials("", sessionConfig.Profile))
		logger.Debug("Session established with a default endpoint", "credFilePath", credFilePath)
	}

	sess, err := session.NewSessionWithOptions(session.Options{
		Config: *config,
	})
	if err != nil {
		return nil, err
	}
	iamSess := iam.New(sess)
	logger.Debug("Authenticating with AWS", "profile", sessionConfig.Profile, "region", sessionConfig.Region, "endpointURL", iamSess.Endpoint, "credFilePath", credFilePath, "iamSess", iamSess)

	devices, err := iamSess.ListMFADevices(&iam.ListMFADevicesInput{})
	if aerr, ok := err.(awserr.Error); ok {
		switch aerr.Code() {
		case "SharedCredsLoad":
			logger.Debug("AWS profile is not valid", "Profile", sessionConfig.Profile)
			return nil, fmt.Errorf("AWS profile is not valid (used `%s`). Please set correct AWS_PROFILE via AWS_PROFILE env var, --aws-profile flag or aws_profile config entry in atun.toml", sessionConfig.Profile)
		default:
			// If the endpoint is localhost (LocalStack) then it's not an error
			if !(strings.Contains(iamSess.Endpoint, "localhost") || strings.Contains(iamSess.Endpoint, "127.0.0.1")) {
				// If endpoint is not related to LocalStack then it's an error
				return nil, err
			}

			logger.Debug("[NO MFA] Using Endpoint: ", iamSess.Endpoint)
		}
	}

	// If there are no MFA devices, return the session
	if len(devices.MFADevices) == 0 {
		return sess, nil
	}

	mfaUpdateRequired, err := isMFAUpdateRequired(sessionConfig.MFASharedCredentialsPath, sessionConfig.Profile)
	if err != nil {
		return nil, err
	}

	if mfaUpdateRequired {
		cred, err := getNewToken(sess, devices.MFADevices[0].SerialNumber)
		if err != nil {
			return nil, err
		}

		mfaCredFile, err := ini.Load(sessionConfig.MFASharedCredentialsPath)
		err = writeCredsToFile(cred, mfaCredFile, sessionConfig.MFASharedCredentialsPath, sessionConfig.Profile)
		if err != nil {
			return nil, err
		}
	}

	// Create a new session with the MFA credentials
	sess, err = session.NewSessionWithOptions(
		session.Options{
			Config:            *aws.NewConfig().WithRegion(sessionConfig.Region),
			Profile:           fmt.Sprintf("%s-mfa", sessionConfig.Profile),
			SharedConfigFiles: []string{sessionConfig.MFASharedCredentialsPath},
		},
	)
	if err != nil {
		return nil, err
	}

	return sess, nil
}

func GetTestSession(c *SessionConfig) (*session.Session, error) {
	sess, err := session.NewSessionWithOptions(
		session.Options{
			Config:  *aws.NewConfig().WithRegion(c.Region),
			Profile: c.Profile,
		},
	)
	if err != nil {
		return nil, err
	}

	return sess, nil
}

func getNewToken(sess *session.Session, serialNumber *string) (*sts.Credentials, error) {
	stsSvc := sts.New(sess)

	mfaCode, err := stscreds.StdinTokenProvider()
	if err != nil {
		return nil, err
	}

	out, err := stsSvc.GetSessionToken(&sts.GetSessionTokenInput{
		SerialNumber: serialNumber,
		TokenCode:    &mfaCode,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get session token: %w", err)
	}

	return out.Credentials, nil
}

func writeCredsToFile(creds *sts.Credentials, f *ini.File, filepath, profile string) error {
	sect, err := f.NewSection(fmt.Sprintf("%s-mfa", profile))
	if err != nil {
		return err
	}

	_, err = sect.NewKey("aws_access_key_id", *creds.AccessKeyId)
	if err != nil {
		return err
	}
	_, err = sect.NewKey("aws_secret_access_key", *creds.SecretAccessKey)
	if err != nil {
		return err
	}
	_, err = sect.NewKey("aws_session_token", *creds.SessionToken)
	if err != nil {
		return err
	}
	_, err = sect.NewKey("token_expiration", creds.Expiration.Format("2006-01-02T15:04:05Z07:00"))
	if err != nil {
		return err
	}

	err = f.SaveTo(filepath)
	if err != nil {
		return err
	}

	return nil
}

func isMFAUpdateRequired(mfaCredFilePath string, profile string) (bool, error) {
	var err error

	updateRequired := false
	mfaCredFile, err := ini.Load(mfaCredFilePath)
	if err != nil {
		mfaCredFile = ini.Empty(ini.LoadOptions{})
		updateRequired = true
	}

	var sect *ini.Section
	var exp *ini.Key

	if !updateRequired {
		sect, err = mfaCredFile.GetSection(fmt.Sprintf("%s-mfa", profile))
		if err != nil {
			updateRequired = true
		}
	}

	if !updateRequired {
		if len(sect.KeyStrings()) != 4 {
			updateRequired = true
		}
	}

	if !updateRequired {
		exp, err = sect.GetKey("token_expiration")
		if err != nil {
			updateRequired = true
		}
	}

	if !updateRequired {
		timeExp, err := time.Parse("2006-01-02T15:04:05Z07:00", exp.String())
		if err != nil {
			updateRequired = true
		}

		if timeExp.Before(time.Now().UTC()) {
			updateRequired = true
		}
	}
	return updateRequired, nil
}

func GetMFASharedCredentialsPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return path.Join(homeDir, ".aws/credentials-mfa"), nil
}
