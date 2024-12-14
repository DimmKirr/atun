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
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/sirupsen/logrus"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"gopkg.in/ini.v1"
)

const (
	path = "/.aws/credentials-mfa"
)

type SessionConfig struct {
	Region      string
	Profile     string
	EndpointUrl string
}

func GetSession(c *SessionConfig) (*session.Session, error) {
	upd := false

	// Check if the env var is set and if not set it to the default value. (Maybe there is a better way to do this?)
	credFilePath := os.Getenv("AWS_SHARED_CREDENTIALS_FILE")
	if credFilePath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		credFilePath = fmt.Sprintf("%s/.aws/credentials", homeDir)
	}

	// Get region from the credentials file if it's not set
	credFile, err := ini.Load(credFilePath)
	if err != nil {
		return nil, err
	}

	profile, err := credFile.GetSection(c.Profile)
	if err != nil {
		return nil, err
	}

	if c.Region == "" {
		c.Region = profile.Key("region").String()
	}

	var config *aws.Config

	if c.EndpointUrl != "" {
		config = aws.NewConfig().WithRegion(c.Region).WithCredentials(credentials.NewSharedCredentials(credFilePath, c.Profile)).WithEndpoint(c.EndpointUrl)
		logger.Debug("Session established", "credFilePath", credFilePath, "endpoint", c.EndpointUrl)
	} else {
		config = aws.NewConfig().WithRegion(c.Region).WithCredentials(credentials.NewSharedCredentials("", c.Profile))
		logger.Debug("Session established with a default endpoint", "credFilePath", credFilePath)
	}

	sess, err := session.NewSessionWithOptions(session.Options{
		Config: *config,
	})
	if err != nil {
		return nil, err
	}
	iamSess := iam.New(sess)
	logger.Debug("Authenticating with AWS", "profile", c.Profile, "region", c.Region, "endpointURL", iamSess.Endpoint, "credFilePath", credFilePath, "iamSess", iamSess)

	devices, err := iamSess.ListMFADevices(&iam.ListMFADevicesInput{})
	if aerr, ok := err.(awserr.Error); ok {
		switch aerr.Code() {
		case "SharedCredsLoad":
			logger.Debug("AWS profile is not valid", "Profile", c.Profile)
			return nil, fmt.Errorf("AWS profile is not valid (used `%s`). Please set correct AWS_PROFILE via AWS_PROFILE env var, --aws-profile flag or aws_profile config entry in atun.toml", c.Profile)
		default:
			// If the endpoint is localhost (LocalStack) then it's not an error
			if !(strings.Contains(iamSess.Endpoint, "localhost") || strings.Contains(iamSess.Endpoint, "127.0.0.1")) {
				// If endpoint is not related to LocalStack then it's an error
				return nil, err
			}

			logrus.Debug("[NO MFA] Using Endpoint: ", iamSess.Endpoint)
		}
	}

	if len(devices.MFADevices) == 0 {
		return sess, nil
	}

	home, _ := os.UserHomeDir()
	filePath := home + path

	credFile, err = ini.Load(filePath)
	if err != nil {
		credFile = ini.Empty(ini.LoadOptions{})
		upd = true
	}

	var sect *ini.Section
	var exp *ini.Key

	if !upd {
		sect, err = credFile.GetSection(fmt.Sprintf("%s-mfa", c.Profile))
		if err != nil {
			upd = true
		}
	}

	if !upd {
		if len(sect.KeyStrings()) != 4 {
			upd = true
		}
	}

	if !upd {
		exp, err = sect.GetKey("token_expiration")
		if err != nil {
			upd = true
		}
	}

	if !upd {
		timeExp, err := time.Parse("2006-01-02T15:04:05Z07:00", exp.String())
		if err != nil {
			upd = true
		}

		if timeExp.Before(time.Now().UTC()) {
			upd = true
		}
	}

	if upd {
		cred, err := getNewToken(sess, devices.MFADevices[0].SerialNumber)
		if err != nil {
			return nil, err
		}

		err = writeCredsToFile(cred, credFile, filePath, c.Profile)
		if err != nil {
			return nil, err
		}
	}

	sess, err = session.NewSessionWithOptions(
		session.Options{
			Config:            *aws.NewConfig().WithRegion(c.Region),
			Profile:           fmt.Sprintf("%s-mfa", c.Profile),
			SharedConfigFiles: []string{filePath},
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

func getNewToken(sess *session.Session, sn *string) (*sts.Credentials, error) {
	stsSvc := sts.New(sess)

	token, err := stscreds.StdinTokenProvider()
	if err != nil {
		return nil, err
	}

	out, err := stsSvc.GetSessionToken(&sts.GetSessionTokenInput{
		SerialNumber: sn,
		TokenCode:    &token,
	})

	if err != nil {
		return nil, err
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
