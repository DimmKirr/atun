/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package aws

import (
	"fmt"
	"github.com/automationd/atun/internal/config"
	"github.com/automationd/atun/internal/logger"
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
	// Load base session using default AWS SDK logic (SSO compatible)
	opts := session.Options{
		SharedConfigState: session.SharedConfigEnable,
		Profile:           sessionConfig.Profile,
	}

	if sessionConfig.Region != "" {
		opts.Config.Region = aws.String(sessionConfig.Region)
	}

	if sessionConfig.EndpointUrl != "" {
		opts.Config.Endpoint = aws.String(sessionConfig.EndpointUrl)
	}

	sess, err := session.NewSessionWithOptions(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS session: %w", err)
	}

	// After the session is built, extract the resolved region
	resolvedRegion := aws.StringValue(sess.Config.Region)
	if resolvedRegion == "" {
		return nil, fmt.Errorf("region could not be resolved from environment or profile")
	}

	// Store it globally
	config.App.Config.AWSRegion = resolvedRegion

	logger.Debug("AWS session created", "profile", sessionConfig.Profile, "region", aws.StringValue(opts.Config.Region), "endpoint", sessionConfig.EndpointUrl)

	identity, _ := sts.New(sess).GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if !strings.Contains(aws.StringValue(identity.Arn), ":user/") {
		logger.Debug("MFA not applicable, principal is not a user", "arn", aws.StringValue(identity.Arn))
		return sess, nil
	}

	// Check for MFA devices (optional if you still want to force MFA usage)
	iamSess := iam.New(sess)
	devices, err := iamSess.ListMFADevices(&iam.ListMFADevicesInput{})
	if err != nil {
		// Allow localhost endpoints to skip IAM
		if !strings.Contains(iamSess.Endpoint, "localhost") && !strings.Contains(iamSess.Endpoint, "127.0.0.1") {
			return nil, fmt.Errorf("failed to list MFA devices: %w", err)
		}
		logger.Debug("LocalStack detected, skipping MFA check")
		return sess, nil
	}

	if len(devices.MFADevices) == 0 {
		// No MFA devices configured
		return sess, nil
	}

	// Check if MFA session needs to be refreshed
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
		if err != nil {
			return nil, fmt.Errorf("failed to load MFA credentials file: %w", err)
		}
		if err := writeCredsToFile(cred, mfaCredFile, sessionConfig.MFASharedCredentialsPath, sessionConfig.Profile); err != nil {
			return nil, err
		}
	}

	// Rebuild session with MFA credentials
	mfaProfile := fmt.Sprintf("%s-mfa", sessionConfig.Profile)

	sess, err = session.NewSessionWithOptions(session.Options{
		Profile:           mfaProfile,
		SharedConfigState: session.SharedConfigEnable,
		SharedConfigFiles: []string{sessionConfig.MFASharedCredentialsPath},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MFA session: %w", err)
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
	return path.Join(homeDir, ".aws", "credentials-mfa"), nil
}
