/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package logger

import (
	"github.com/spf13/viper"
	"log/slog"
	"os"
	"strings"
)

var defaultLogger *slog.Logger

// Initialize sets up the logger with a specified log level
func Initialize(logLevel string) {
	// Map log levels from configuration to slog
	var slogLevel slog.Level
	switch strings.ToLower(logLevel) {
	case "info":
		slogLevel = slog.LevelInfo
	case "debug":
		slogLevel = slog.LevelDebug
	case "trace":
		slogLevel = slog.LevelDebug // slog doesn't have a trace level
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	case "fatal":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}

	// Configure slog with a text handler
	opts := slog.HandlerOptions{Level: slogLevel}
	handler := slog.NewTextHandler(os.Stdout, &opts)
	defaultLogger = slog.New(handler)

	// Log initialization message
	defaultLogger.Debug("Logger initialized", "level", slogLevel.String())
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

func init() {
	// Initialize the logger with the default log level
	Initialize(viper.GetString("LOG_LEVEL"))
}
