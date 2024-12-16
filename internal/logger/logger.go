/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package logger

import (
	"github.com/pterm/pterm"
	"github.com/spf13/viper"
	"log/slog"
	"os"
	"strings"
	"time"
)

var defaultLogger *slog.Logger

// ApplyPtermTheme applies custom styles to pterm
func ApplyPtermTheme() {
	// Customize INFO prefix
	pterm.Info.Prefix = pterm.Prefix{
		Text:  "ℹ️",                                     // Custom prefix text
		Style: pterm.NewStyle(pterm.FgCyan, pterm.Bold), // Cyan + Bold
	}

	// Customize WARNING prefix
	pterm.Warning.Prefix = pterm.Prefix{
		Text:  "⚠️",
		Style: pterm.NewStyle(pterm.FgYellow, pterm.Bold),
	}

	// Customize SUCCESS prefix
	pterm.Success.Prefix = pterm.Prefix{
		Text:  "✅",
		Style: pterm.NewStyle(pterm.FgGreen, pterm.Bold),
	}

	// Customize ERROR prefix
	pterm.Error.Prefix = pterm.Prefix{
		Text:  "❌",
		Style: pterm.NewStyle(pterm.FgRed, pterm.Bold),
	}

	// Customize DEBUG prefix (no timestamp)
	pterm.Debug.Prefix = pterm.Prefix{
		Text:  "🐞 DEBUG",
		Style: pterm.NewStyle(pterm.FgMagenta), // Magenta text for debug
	}

}

// Initialize sets up the logger with a specified log level
func Initialize(logLevel string, logPlainText bool) {

	// Map log levels from configuration to slog
	var slogLevel slog.Level
	switch strings.ToLower(logLevel) {
	case "debug":
		slogLevel = slog.LevelDebug
	case "info":
		slogLevel = slog.LevelInfo
	case "warning":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	case "fatal":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}

	// Configure slog with a text handler
	handler := pterm.NewSlogHandler(&pterm.DefaultLogger)
	pterm.DefaultLogger.Level = pterm.LogLevel(slogLevel)
	if !logPlainText {
		// Use text-only logging style
		ApplyPtermTheme()
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
func Success(msg string) {
	if viper.GetBool("QUIET") {
		return
	}

	pterm.Success.Println(msg)
}

// StartCustomSpinner creates and starts a fresh custom spinner
func StartCustomSpinner(message string) *pterm.SpinnerPrinter {
	// Clone DefaultSpinner as a new variable
	spinner := pterm.DefaultSpinner // Direct assignment to create a new copy

	// Customize the spinner
	spinner.Sequence = []string{
		"    🐟",
		"   🐟 ",
		"  🐟  ",
		" 🐟   ",
		"🐟    ",
		//"🫧   ",
		//" 🫧  ",
		//"  🫧 ",
		//"   🫧",
	}

	spinner.Style = pterm.NewStyle(pterm.FgCyan) // Custom color
	spinner.Delay = 150 * time.Millisecond       // Frame delay

	// Start the spinner
	s, _ := spinner.Start(message)
	return s
}

func init() {
	// Initialize the logger with the default log level
	Initialize(viper.GetString("LOG_LEVEL"), false)

}
