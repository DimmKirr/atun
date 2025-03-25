/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package logger

import (
	"fmt"
	"github.com/spf13/viper"
	"log/slog"
	"os"
	"strings"

	"github.com/pterm/pterm"
)

var defaultLogger *slog.Logger

// Initialize sets up the logger with a specified log level
func Initialize(logLevel string, logPlainText bool) {
	// Map log levels from configuration to pterm log levels
	var ptermLogLevel pterm.LogLevel
	switch strings.ToLower(logLevel) {
	case "debug":
		ptermLogLevel = pterm.LogLevelDebug
	case "info":
		ptermLogLevel = pterm.LogLevelInfo
	case "warning":
		ptermLogLevel = pterm.LogLevelWarn
	case "error":
		ptermLogLevel = pterm.LogLevelError
	case "fatal":
		ptermLogLevel = pterm.LogLevelError
	default:
		ptermLogLevel = pterm.LogLevelInfo
	}

	// Configure slog with a text handler
	handler := pterm.NewSlogHandler(&pterm.DefaultLogger)

	pterm.DefaultLogger.Level = ptermLogLevel
	if !logPlainText {
		// Use text-only logging style
		ApplyPtermTheme(0)
	}

	// Create a new slog logger with the handler
	defaultLogger = slog.New(handler)
}

// Info logs an info message
func Info(msg string, keysAndValues ...interface{}) {
	defaultLogger.Info(msg, keysAndValues...)
}

// Debug logs a debug message
func Debug(msg string, keysAndValues ...interface{}) {
	defaultLogger.Debug(msg, keysAndValues...)
}

// Warn logs a warning message
func Warn(msg string, keysAndValues ...interface{}) {
	defaultLogger.Warn(msg, keysAndValues...)
}

// Error logs an error message
func Error(msg string, keysAndValues ...interface{}) {
	defaultLogger.Error(msg, keysAndValues...)
}

// Fatal logs a fatal error message and exits the application
func Fatal(msg string, keysAndValues ...interface{}) {
	defaultLogger.Error(msg, keysAndValues...)
	os.Exit(1)
}

// Success prints a user-facing success message with optional centralized control
func Success(msg string, keysAndValues ...interface{}) {
	defaultLogger.Info(msg, keysAndValues...)
}

func init() {
	Initialize(viper.GetString("LOG_LEVEL"), false)
}

// ApplyPtermTheme applies custom styles to pterm
func ApplyPtermTheme(indent int) {
	// Customize INFO prefix

	indentLevel := strings.Repeat(" ", indent)

	pterm.Info.Prefix = pterm.Prefix{
		Text:  fmt.Sprintf("%sℹ", indentLevel),          // Custom prefix text
		Style: pterm.NewStyle(pterm.FgCyan, pterm.Bold), // Cyan + Bold
	}

	// Customize WARNING prefix
	pterm.Warning.Prefix = pterm.Prefix{
		Text:  fmt.Sprintf(`%s⚠`, indentLevel),
		Style: pterm.NewStyle(pterm.FgYellow, pterm.Bold),
	}

	// Customize SUCCESS prefix
	pterm.Success.Prefix = pterm.Prefix{
		//Text: "",
		//Style: nil,
		Text:  fmt.Sprintf("%s✔", indentLevel),
		Style: pterm.NewStyle(pterm.FgLightGreen, pterm.Bold),
	}

	// Customize ERROR prefix
	pterm.Error.Prefix = pterm.Prefix{
		Text:  fmt.Sprintf("%s⨯", indentLevel),
		Style: pterm.NewStyle(pterm.FgRed, pterm.Bold),
	}

	// Customize DEBUG prefix (no timestamp)
	pterm.Debug.Prefix = pterm.Prefix{
		Text:  fmt.Sprintf("%s⚙︎", indentLevel),
		Style: pterm.NewStyle(pterm.FgMagenta), // Magenta text for debug
	}
}
