/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: © 2024 Dmitry Kireev
 */

package logger

import (
	"github.com/pterm/pterm"
	"github.com/pterm/pterm/putils"
	"math/rand"
	"time"
)

const (
	width     = 50 // Total width of the terminal
	height    = 5  // Number of rows for fish animation
	frameRate = 110 * time.Millisecond
)

// Fish struct to hold fish position and row
type Fish struct {
	Pos      int // Horizontal position
	Row      int // Vertical position (y)
	StartPos int // Initial starting position
}

// RenderAsciiArt animates big text (50x10) and fishes moving right-to-left below it
func RenderAsciiArt() {
	// Define initial positions of the fishes
	fishes := []Fish{
		{Pos: 10, Row: 0, StartPos: 10},
		{Pos: 25, Row: 1, StartPos: 25},
		{Pos: 30, Row: 2, StartPos: 30},
		{Pos: 15, Row: 3, StartPos: 15},
		{Pos: 50, Row: 4, StartPos: 50},
	}

	// Start two separate areas
	bigTextArea, _ := pterm.DefaultArea.WithCenter(false).Start()
	fishArea, _ := pterm.DefaultArea.WithCenter(false).Start()

	defer bigTextArea.Stop()
	defer fishArea.Stop()

	// Render the big text "Atun" once

	bigText, _ := pterm.DefaultBigText.WithLetters(
		putils.LettersFromStringWithStyle("-", pterm.FgGray.ToStyle()),
		putils.LettersFromStringWithStyle("Atun", pterm.FgLightCyan.ToStyle()),
		putils.LettersFromStringWithStyle("-", pterm.FgGray.ToStyle()),
	).Srender() // Render the big text to the terminal
	bigTextArea.Update(bigText) // Render big text area once
	bigTextArea.Update(bigText + "\n")

	//oceanBed := generateBed()

	// Infinite Animation Loop for the fish
	for {
		// Generate the fish animation frame
		fishFrame := generateFrame(fishes, width, height)
		//fishFrame += oceanBed

		// Combine areas: BigText is rendered above the fish animation
		fishArea.Update(fishFrame)
		time.Sleep(frameRate)

		// Update positions for all fish
		for i := range fishes {
			variation := rand.Intn(4) + 1

			if fishes[i].Pos > 0 {
				fishes[i].Pos = fishes[i].Pos - 1*variation
			} else {
				// Reset position to the far-right side of the screen
				fishes[i].Pos = width - 1
			}
		}
	}
}

// generateFrame generates a frame with multiple fishes at their positions
func generateFrame(fishes []Fish, width int, height int) string {
	var output string

	// Define the number of bars for each row
	barPattern := []int{3, 2, 1, 2, 3}

	for y := 0; y < height; y++ {
		line := ""

		// Add bars based on the bar pattern
		for i := 0; i < barPattern[y%len(barPattern)]; i++ { // Repeats pattern if needed
			line += "█"
		}

		// Add spaces to fill up the rest of the left boundary
		for i := barPattern[y%len(barPattern)]; i < 3; i++ {
			line += " "
		}

		// Generate fish animation to the right of the bars
		for x := 3; x < width; x++ { // Start from 3 since left space is filled
			char := " "
			for _, fish := range fishes {
				if y == fish.Row && x == fish.Pos {
					char = "🐟"
					break
				}
			}
			line += char
		}

		output += line + "\n"
	}

	return output
}

// generateBed takes ocean bed emojis and randomly generates a string consisting of them based on the width specified by the user
func generateBed() string {
	var oceanBed string
	// Set list of emojis for the ocean bed to randomly select from
	oceanBedEmojis := []string{
		"🪸",
		"🪸",
		"🪨",
		"🪨",
		"🪨",
		"🐚",
		"🦑",
		"🌿",
		"🌿",
		"🌿",
		"🌾",
	}

	// Generate a random number between min and max length of ocianBedEmojis
	rand.Seed(time.Now().UnixNano())

	// use width constant and build a loop to create a final string
	for i := 0; i < width/2; i++ {
		randomEmoji := rand.Intn(len(oceanBedEmojis))
		// Print random emoji by
		oceanBed += oceanBedEmojis[randomEmoji]
	}

	return oceanBed
}
