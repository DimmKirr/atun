/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2025 Dmitry Kireev
 */

package ux

import (
	"fmt"
	"github.com/automationd/atun/internal/config"
	"github.com/automationd/atun/internal/logger"
	"github.com/automationd/atun/internal/ssh"
	"github.com/pterm/pterm"
	"io"
	"regexp"
	"strings"
	"time"
)

// ProgressSpinner is a wrapper around pterm.SpinnerPrinter that
// automatically falls back to logging when spinners are disabled
type ProgressSpinner struct {
	spinner *pterm.SpinnerPrinter
}

type MultiSpinner struct {
	spinner *pterm.MultiPrinter
}

// StartCustomSpinner creates and starts a fresh custom spinner
func StartMainSpinner(message string, writer io.Writer) *pterm.SpinnerPrinter {
	// Clone DefaultSpinner as a new variable
	spinner := pterm.DefaultSpinner.WithWriter(writer) // Direct assignment to create a new copy

	// Customize the spinner
	spinner.Sequence = []string{
		//" ⣾ ",
		//" ⣽ ",
		//" ⣻ ",
		//" ⢿ ",
		//" ⡿ ",
		//" ⣟ ",
		//" ⣯ ",
		//" ⣷ ",
		//" » ",
		//" ▸ ",
		" 🐟",
	}

	spinner.Style = pterm.NewStyle(pterm.FgCyan) // Custom color
	spinner.Delay = 150 * time.Millisecond       // Frame delay

	// Start the spinner
	s, _ := spinner.Start(message)
	return s
}

func StartSubSpinner(message string, writer io.Writer) *pterm.SpinnerPrinter {
	spinner := pterm.DefaultSpinner.
		WithWriter(writer)

	// Customize the spinner
	spinner.Sequence = []string{
		" ⡀ ", // Frame 1: (y1) - Bottom
		" ⢀ ", // Frame 2: (y2)
		" ⠄ ", // Frame 3: (y3)
		" ⠠ ", // Frame 4: (y4)
		" ⠐ ", // Frame 5: (y5)
		" ⠂ ", // Frame 6: (y6)
		" ⠁ ", // Frame 7: (y7)
		" ⠈ ", // Frame 8: (y8) - Top
		"   ", // Frame 9:
		"   ", // Frame 10
		//"⠁  ",
		//" ⠂ ",
		//"  ⠄",
		//" ⡀ ",
		//"⢀  ",
		//" ⠠ ",
		//" ⠐ ",
		//"⠈  ",
		//⠁⠂⠄⡀⢀⠠⠐⠈

		//" ⣾ ",
		//" ⣽ ",
		//" ⣻ ",
		//" ⢿ ",
		//" ⡿ ",
		//" ⣟ ",
		//" ⣯ ",
		//" ⣷ ",
		//"▁▁▁",
		//"▃▃▃",
		//"▄▄▄",
		//"▅▅▅",
		//"▅▅▅",
		//"▇▇▇",
		//"███",
		//"▇▇▇",
		//"▆▆▆",
		//"▅▅▅",
		//"▄▄▄",
		//"▃▃▃",
	}
	spinner.MessageStyle = pterm.NewStyle(pterm.Concealed)

	spinner.Style = pterm.NewStyle(
		pterm.FgLightBlue,
	) // Custom color
	spinner.Delay = 150 * time.Millisecond // Frame delay

	// Start the spinner
	s, _ := spinner.Start(message)
	return s
}

func NewMainSpinner(message string, writer io.Writer) *ProgressSpinner {
	ps := &ProgressSpinner{}

	if !config.App.Config.LogPlainText {
		ps.spinner = StartMainSpinner(message, writer)
	} else {
		logger.Info(message)
	}

	return ps
}

func NewSubSpinner(message string, writer io.Writer) *ProgressSpinner {
	ps := &ProgressSpinner{}

	if !config.App.Config.LogPlainText {
		ps.spinner = StartSubSpinner(message, writer)
	} else {
		logger.Info(message)
	}

	return ps
}

// NewProgressSpinner creates a new progress spinner or logs a message
// if spinners are disabled
func NewProgressSpinner(message string) *ProgressSpinner {
	ps := &ProgressSpinner{}

	// TODO: make it without side-effects at some point
	if !config.App.Config.LogPlainText {
		// Clone DefaultSpinner as a new variable

		spinner := pterm.DefaultSpinner // Direct assignment to create a new copy

		// Customize the spinner
		spinner.Sequence = []string{
			" ⡀ ", // Frame 1: (y1) - Bottom
			" ⢀ ", // Frame 2: (y2)
			" ⠄ ", // Frame 3: (y3)
			" ⠠ ", // Frame 4: (y4)
			" ⠐ ", // Frame 5: (y5)
			" ⠂ ", // Frame 6: (y6)
			" ⠁ ", // Frame 7: (y7)
			" ⠈ ", // Frame 8: (y8) - Top
			"   ", // Frame 9:
			"   ", // Frame 10
		}

		spinner.Style = pterm.NewStyle(pterm.FgLightBlue)
		spinner.Delay = 150 * time.Millisecond

		ps.spinner, _ = spinner.Start(message)

	} else {
		logger.Info(message)
	}

	return ps
}

// UpdateText updates the spinner text or logs the message
func (ps *ProgressSpinner) UpdateText(message string, keysAndValues ...interface{}) *ProgressSpinner {
	if ps.spinner != nil {
		ps.spinner.UpdateText(message)
	} else {
		logger.Info(message, keysAndValues...)
	}
	return ps
}

// Success marks the spinner as successful or logs a success message
func (ps *ProgressSpinner) Success(message string, keysAndValues ...interface{}) *ProgressSpinner {
	if ps.spinner != nil {
		ps.spinner.Success(message)
	} else {
		logger.Success(message, keysAndValues...)
	}
	return ps
}

// Pause pauses the spinner or logs a message
func (ps *ProgressSpinner) Pause() *ProgressSpinner {
	if ps.spinner != nil {
		ps.spinner.IsActive = false
	}
	return ps
}

// Pause pauses the spinner or logs a message
func (ps *ProgressSpinner) Stop() *ProgressSpinner {
	if ps.spinner != nil {
		ps.spinner.Stop()
	}
	return ps
}

func (ps *ProgressSpinner) Fail(message string, keysAndValues ...interface{}) *ProgressSpinner {
	if ps.spinner != nil {
		ps.spinner.Fail(message)
	} else {
		logger.Error(message, keysAndValues...)
	}
	return ps
}

// Warning updates the spinner with a warning or logs a warning message
func (ps *ProgressSpinner) Warning(message string) *ProgressSpinner {
	if ps.spinner != nil {
		ps.spinner.Warning(message)
	} else {
		logger.Warn(message)
	}
	return ps
}

// RenderConnectionsTable renders a table with the connections
func (ps *ProgressSpinner) Status(message string, tunnelIsUp bool, endpoints []ssh.Endpoint) {
	if ps.spinner != nil {
		err := RenderEndpointsTable(endpoints)
		if err != nil {
			logger.Info(fmt.Sprintln(message))
			logger.Error("Error rendering connections table", "error", err, "connections", endpoints)
		}

	} else {
		logger.Info(fmt.Sprintln(message), "tunnelIsUp", tunnelIsUp, "connections", endpoints)
	}
}

// Println prints pterm or logger depending on the config
func Println(message string) {
	if !config.App.Config.LogPlainText {
		pterm.Println(fmt.Sprintf("\n 🐟 %s", message))
	} else {
		logger.Info(message)
	}
}

//// NewMainSpinner creates a new multi spinner or logs a message if spinners are disabled
//func NewMainSpinner(message string, multi pterm.MultiPrinter) pterm.SpinnerPrinter {
//	//ms := &MultiSpinner{}
//
//	if !config.App.Config.LogPlainText {
//		ms := pterm.DefaultSpinner.WithWriter(multi.NewWriter())
//
//		// Customize the spinner
//		ms.Sequence = []string{
//			"    🐟",
//			"   🐟 ",
//			"  🐟  ",
//			" 🐟   ",
//			"🐟    ",
//			"🫧    ",
//		}
//
//		ms.Style = pterm.NewStyle(pterm.FgCyan) // Custom color
//		ms.Delay = 150 * time.Millisecond       // Frame delay
//
//		// Start the spinner
//		s, _ := ms.Start(message)
//		return s
//	} else {
//		logger.Info(message)
//	}
//
//}

//func (ms MultiSpinner) Success(message string) {
//	if ms.spinner != nil {
//		ms.spinner.Success(message)
//		ms.spinner.Stop()
//	}
//}

func ClearLines(linesNumber int) {
	for i := 0; i < linesNumber; i++ {
		fmt.Print("\033[1A") // Move cursor up 1 line
		fmt.Print("\033[K")  // Erase the line
	}

}

// RenderEndpointsTable creates and renders a custom table with a given header and rows.
func RenderEndpointsTable(endpoints []ssh.Endpoint) error {
	terminalWidth := pterm.GetTerminalWidth()

	statusHeaderLabel := "Status"
	remoteHeaderLabel := "Remote (Cloud)"
	localHeaderLabel := "Local"
	upStatusLabel := "  UP  "
	downStatusLabel := " DOWN "

	if terminalWidth < 45 {
		statusHeaderLabel = " ◉ "
		remoteHeaderLabel = "Remote"
		localHeaderLabel = "Local"
		upStatusLabel = " ▶ ︎"
		downStatusLabel = " ⏹ ︎"
	}

	var rows [][]string
	var remoteRowMaxLength int
	var localRowMaxLength int

	for _, endpoint := range endpoints {
		// Construct each field separately to measure actual lengths
		statusCol := pterm.NewStyle(
			pterm.FgBlack,
			pterm.Bold,
			pterm.BgGreen,
		).Sprint(upStatusLabel)
		if !endpoint.Status {
			statusCol = pterm.NewStyle(
				pterm.FgLightWhite,
				pterm.BgRed,
				pterm.Bold,
			).Sprint(downStatusLabel)
		}

		localCol := fmt.Sprintf("%s:%d", endpoint.LocalHost, endpoint.LocalPort)
		fullRemoteCol := fmt.Sprintf("%s:%v", endpoint.RemoteHost, endpoint.RemotePort)

		// Measure actual column widths
		statusWidth := len(stripANSI(statusCol))
		localWidth := len(localCol)
		remoteWidth := len(fullRemoteCol)

		// Estimated table width including separators and some padding
		// Approximate padding between columns
		var padding int
		if terminalWidth < 45 {
			padding = 10
		} else {
			padding = 14
		}

		estimatedWidth := statusWidth + remoteWidth + localWidth + padding

		// If the table is too wide, shorten Remote column
		remoteCol := fullRemoteCol
		if estimatedWidth > terminalWidth {
			availableRemoteWidth := max(10, terminalWidth-statusWidth-localWidth-padding)
			if len(endpoint.RemoteHost) > availableRemoteWidth-6 { // Allow space for `...`
				remoteCol = fmt.Sprintf("%s...:%v", endpoint.RemoteHost[:availableRemoteWidth-6], endpoint.RemotePort)
			}
		}

		// If terminal is very narrow, shorten Local column as well
		localColFinal := localCol
		if terminalWidth < 60 {
			availableLocalWidth := max(5, terminalWidth-statusWidth-len(remoteCol)-padding)
			if len(endpoint.LocalHost) > availableLocalWidth-10 {
				localColFinal = fmt.Sprintf("%s...:%d", endpoint.LocalHost[:max(availableLocalWidth-10, 0)], endpoint.LocalPort)
			}
		}
		if len(remoteCol) > remoteRowMaxLength {
			remoteRowMaxLength = len(remoteCol)
		}

		if len(localColFinal) > localRowMaxLength {
			localRowMaxLength = len(localColFinal)
		}

		row := []string{statusCol, remoteCol, localColFinal}
		rows = append(rows, row)
	}

	centeredRemoteHeaderLabel := centerText(remoteHeaderLabel, remoteRowMaxLength)
	centeredLocalHeaderLabel := centerText(localHeaderLabel, localRowMaxLength)

	header := []string{statusHeaderLabel, centeredRemoteHeaderLabel, centeredLocalHeaderLabel}

	// Set table data (header + rows)
	data := append([][]string{header}, rows...)

	table := pterm.DefaultTable.
		WithHasHeader().
		WithRowSeparator("─").
		WithHeaderRowSeparator("─")

	// Render the table
	tableStr, err := table.WithData(data).Srender()
	if err != nil {
		return err
	}

	// Small/large term "reactive" styling
	var tableTitle string
	var tableLeftPadding int
	var tableRightPadding int
	if terminalWidth < 45 {
		tableTitle = fmt.Sprintf("%s - %s",
			pterm.NewStyle(pterm.FgLightWhite).Sprint(config.App.Config.AWSProfile),
			pterm.NewStyle(pterm.FgLightWhite).Sprint(config.App.Config.Env))
		tableLeftPadding = 1
		tableRightPadding = 1
	} else {
		tableTitle = fmt.Sprintf("%s - %s - %s",
			pterm.NewStyle(pterm.FgLightWhite).Sprint(config.App.Config.AWSProfile),
			pterm.NewStyle(pterm.FgLightWhite).Sprint(config.App.Config.Env),
			pterm.NewStyle(pterm.FgLightWhite).Sprint(config.App.Config.RouterHostID))
		tableLeftPadding = 2
		tableRightPadding = 2
	}

	// Print the table inside a box
	pterm.DefaultBox.
		WithTitle(tableTitle).
		WithTitleBottomRight().
		WithLeftPadding(tableLeftPadding).
		WithRightPadding(tableRightPadding).
		WithBottomPadding(0).
		Println(tableStr)

	return nil
}

// Helper function to strip ANSI escape codes from a string for correct width calculation
func stripANSI(input string) string {
	re := regexp.MustCompile(`\x1B\[[0-9;]*[mK]`)
	return re.ReplaceAllString(input, "")
}

// Helper function to get the max of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func centerText(text string, totalWidth int) string {
	textLength := len(text)
	if textLength >= totalWidth {
		return text // If text is longer, return as is
	}

	padding := totalWidth - textLength
	leftPadding := padding / 2
	rightPadding := padding - leftPadding // Ensures minimal difference
	result := strings.Repeat(" ", leftPadding) + text + strings.Repeat(" ", rightPadding)
	return result
}

func GetConfirmation(message string, defaultValues ...bool) (bool, error) {
	// Set default value to true if not provided
	defaultValue := true
	if len(defaultValues) > 0 {
		defaultValue = defaultValues[0]
	}

	prefixedMessage := fmt.Sprintf(" %s  %s", pterm.LightBlue("?"), message)
	return pterm.DefaultInteractiveConfirm.
		WithDefaultText(prefixedMessage).
		WithConfirmStyle(pterm.NewStyle(pterm.FgGreen)).
		WithRejectStyle(pterm.NewStyle(pterm.FgRed)).
		WithDefaultValue(defaultValue).
		WithTextStyle(pterm.NewStyle(pterm.FgLightWhite)).
		WithSuffixStyle(pterm.NewStyle(pterm.FgBlue)).
		Show()
}

func GetTextInput(message string, defaultValues ...string) (string, error) {

	defaultValue := ""
	if len(defaultValues) > 0 {
		defaultValue = defaultValues[0]
	}

	prefixedMessage := fmt.Sprintf(" %s  %s", pterm.LightBlue("?"), message)
	return pterm.DefaultInteractiveTextInput.
		WithDefaultText(prefixedMessage).
		WithDefaultValue(defaultValue).
		Show()
}

func GetInteractiveSelection(message string, options []string, defaultValues ...string) (string, error) {
	prefix := "   "

	defaultValue := ""
	if len(defaultValues) > 0 {
		defaultValue = prefix + defaultValues[0]
	}

	for i := range options {
		options[i] = prefix + options[i]
	}

	prefixedMessage := fmt.Sprintf(" %s  %s", pterm.LightBlue("?"), message)

	result, err := pterm.DefaultInteractiveSelect.
		WithDefaultText(prefixedMessage).
		WithOptions(options).
		WithDefaultOption(defaultValue).
		Show()

	result = strings.TrimPrefix(result, prefix)

	return result, err
}
