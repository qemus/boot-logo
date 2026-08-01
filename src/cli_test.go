package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseOptionsDefaultReplace(t *testing.T) {
	result, handled, err := parseOptions([]string{
		"logo.bmp",
		"firmware.fd",
	})
	if err != nil {
		t.Fatalf("parseOptions() returned an error: %v", err)
	}

	if handled {
		t.Fatal("parseOptions() unexpectedly handled the command")
	}

	if result.command != commandReplace {
		t.Errorf(
			"command = %q, want %q",
			result.command,
			commandReplace,
		)
	}

	if result.imagePath != "logo.bmp" {
		t.Errorf(
			"imagePath = %q, want %q",
			result.imagePath,
			"logo.bmp",
		)
	}

	if result.firmwarePath != "firmware.fd" {
		t.Errorf(
			"firmwarePath = %q, want %q",
			result.firmwarePath,
			"firmware.fd",
		)
	}

	if result.outputPath != "firmware.boot-logo.fd" {
		t.Errorf(
			"outputPath = %q, want %q",
			result.outputPath,
			"firmware.boot-logo.fd",
		)
	}

	if result.inPlace {
		t.Fatal("inPlace = true, want false")
	}
}

func TestParseOptionsExplicitReplace(t *testing.T) {
	result, handled, err := parseOptions([]string{
		"replace",
		"--output",
		"modified.fd",
		"logo.bmp",
		"firmware.fd",
	})
	if err != nil {
		t.Fatalf("parseOptions() returned an error: %v", err)
	}

	if handled {
		t.Fatal("parseOptions() unexpectedly handled the command")
	}

	if result.command != commandReplace {
		t.Errorf(
			"command = %q, want %q",
			result.command,
			commandReplace,
		)
	}

	if result.outputPath != "modified.fd" {
		t.Errorf(
			"outputPath = %q, want %q",
			result.outputPath,
			"modified.fd",
		)
	}
}

func TestParseOptionsOutputEqualsSyntax(t *testing.T) {
	result, handled, err := parseOptions([]string{
		"--output=modified.fd",
		"logo.bmp",
		"firmware.fd",
	})
	if err != nil {
		t.Fatalf("parseOptions() returned an error: %v", err)
	}

	if handled {
		t.Fatal("parseOptions() unexpectedly handled the command")
	}

	if result.outputPath != "modified.fd" {
		t.Errorf(
			"outputPath = %q, want %q",
			result.outputPath,
			"modified.fd",
		)
	}
}

func TestParseOptionsInPlace(t *testing.T) {
	result, handled, err := parseOptions([]string{
		"--in-place",
		"logo.bmp",
		"firmware.fd",
	})
	if err != nil {
		t.Fatalf("parseOptions() returned an error: %v", err)
	}

	if handled {
		t.Fatal("parseOptions() unexpectedly handled the command")
	}

	if !result.inPlace {
		t.Fatal("inPlace = false, want true")
	}

	if result.outputPath != result.firmwarePath {
		t.Errorf(
			"outputPath = %q, want firmware path %q",
			result.outputPath,
			result.firmwarePath,
		)
	}
}

func TestParseOptionsExtract(t *testing.T) {
	result, handled, err := parseOptions([]string{
		"extract",
		"firmware.fd",
	})
	if err != nil {
		t.Fatalf("parseOptions() returned an error: %v", err)
	}

	if handled {
		t.Fatal("parseOptions() unexpectedly handled the command")
	}

	if result.command != commandExtract {
		t.Errorf(
			"command = %q, want %q",
			result.command,
			commandExtract,
		)
	}

	if result.firmwarePath != "firmware.fd" {
		t.Errorf(
			"firmwarePath = %q, want %q",
			result.firmwarePath,
			"firmware.fd",
		)
	}

	if result.outputPath != "firmware.fd.logo.bmp" {
		t.Errorf(
			"outputPath = %q, want %q",
			result.outputPath,
			"firmware.fd.logo.bmp",
		)
	}
}

func TestParseOptionsExtractOutput(t *testing.T) {
	result, handled, err := parseOptions([]string{
		"extract",
		"--output",
		"logo.bmp",
		"firmware.fd",
	})
	if err != nil {
		t.Fatalf("parseOptions() returned an error: %v", err)
	}

	if handled {
		t.Fatal("parseOptions() unexpectedly handled the command")
	}

	if result.outputPath != "logo.bmp" {
		t.Errorf(
			"outputPath = %q, want %q",
			result.outputPath,
			"logo.bmp",
		)
	}
}

func TestParseOptionsDoubleDash(t *testing.T) {
	result, handled, err := parseOptions([]string{
		"--",
		"-logo.bmp",
		"-firmware.fd",
	})
	if err != nil {
		t.Fatalf("parseOptions() returned an error: %v", err)
	}

	if handled {
		t.Fatal("parseOptions() unexpectedly handled the command")
	}

	if result.imagePath != "-logo.bmp" {
		t.Errorf(
			"imagePath = %q, want %q",
			result.imagePath,
			"-logo.bmp",
		)
	}

	if result.firmwarePath != "-firmware.fd" {
		t.Errorf(
			"firmwarePath = %q, want %q",
			result.firmwarePath,
			"-firmware.fd",
		)
	}
}

func TestParseOptionsHelp(t *testing.T) {
	var output bytes.Buffer

	previousStdout := stdout
	stdout = &output
	t.Cleanup(func() {
		stdout = previousStdout
	})

	_, handled, err := parseOptions([]string{"--help"})
	if err != nil {
		t.Fatalf("parseOptions() returned an error: %v", err)
	}

	if !handled {
		t.Fatal("parseOptions() did not handle --help")
	}

	if !strings.Contains(output.String(), "Usage:") {
		t.Fatalf(
			"help output does not contain usage information:\n%s",
			output.String(),
		)
	}
}

func TestParseOptionsVersion(t *testing.T) {
	var output bytes.Buffer

	previousStdout := stdout
	previousVersion := Version

	stdout = &output
	Version = "1.2.3"

	t.Cleanup(func() {
		stdout = previousStdout
		Version = previousVersion
	})

	_, handled, err := parseOptions([]string{"--version"})
	if err != nil {
		t.Fatalf("parseOptions() returned an error: %v", err)
	}

	if !handled {
		t.Fatal("parseOptions() did not handle --version")
	}

	if output.String() != "1.2.3\n" {
		t.Errorf(
			"version output = %q, want %q",
			output.String(),
			"1.2.3\n",
		)
	}
}

func TestParseOptionsRejectsUnknownOption(t *testing.T) {
	_, _, err := parseOptions([]string{
		"--unknown",
		"logo.bmp",
		"firmware.fd",
	})

	if err == nil {
		t.Fatal("parseOptions() accepted an unknown option")
	}
}

func TestParseOptionsRejectsMissingOutputPath(t *testing.T) {
	_, _, err := parseOptions([]string{
		"--output",
	})

	if err == nil {
		t.Fatal("parseOptions() accepted --output without a path")
	}
}

func TestParseOptionsRejectsOutputAndInPlace(t *testing.T) {
	_, _, err := parseOptions([]string{
		"--in-place",
		"--output",
		"modified.fd",
		"logo.bmp",
		"firmware.fd",
	})

	if err == nil {
		t.Fatal(
			"parseOptions() accepted --in-place together with --output",
		)
	}
}

func TestParseOptionsRejectsImplicitOverwrite(t *testing.T) {
	firmwarePath := filepath.Join(
		t.TempDir(),
		"firmware.fd",
	)

	_, _, err := parseOptions([]string{
		"--output",
		firmwarePath,
		"logo.bmp",
		firmwarePath,
	})

	if err == nil {
		t.Fatal(
			"parseOptions() accepted overwriting the firmware without --in-place",
		)
	}
}

func TestParseOptionsRejectsExtractInPlace(t *testing.T) {
	_, _, err := parseOptions([]string{
		"extract",
		"--in-place",
		"firmware.fd",
	})

	if err == nil {
		t.Fatal("parseOptions() accepted --in-place with extract")
	}
}

func TestParseOptionsRejectsMissingReplaceArguments(t *testing.T) {
	var output bytes.Buffer

	previousStderr := stderr
	stderr = &output
	t.Cleanup(func() {
		stderr = previousStderr
	})

	_, _, err := parseOptions([]string{
		"logo.bmp",
	})

	if err == nil {
		t.Fatal("parseOptions() accepted missing replace arguments")
	}

	if !strings.Contains(output.String(), "Usage:") {
		t.Fatal("missing arguments did not print usage information")
	}
}

func TestParseOptionsRejectsMissingExtractArgument(t *testing.T) {
	var output bytes.Buffer

	previousStderr := stderr
	stderr = &output
	t.Cleanup(func() {
		stderr = previousStderr
	})

	_, _, err := parseOptions([]string{
		"extract",
	})

	if err == nil {
		t.Fatal("parseOptions() accepted a missing firmware path")
	}

	if !strings.Contains(output.String(), "Usage:") {
		t.Fatal("missing firmware path did not print usage information")
	}
}

func TestDefaultReplaceOutput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "extension",
			input:    "firmware.fd",
			expected: "firmware.boot-logo.fd",
		},
		{
			name:     "no extension",
			input:    "firmware",
			expected: "firmware.boot-logo",
		},
		{
			name:     "multiple extensions",
			input:    "firmware.code.fd",
			expected: "firmware.code.boot-logo.fd",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual := defaultReplaceOutput(test.input)

			if actual != test.expected {
				t.Errorf(
					"defaultReplaceOutput(%q) = %q, want %q",
					test.input,
					actual,
					test.expected,
				)
			}
		})
	}
}

func TestSamePath(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "firmware.fd")

	if !samePath(path, filepath.Clean(path)) {
		t.Fatal("samePath() returned false for identical paths")
	}

	if samePath(
		path,
		filepath.Join(directory, "other.fd"),
	) {
		t.Fatal("samePath() returned true for different paths")
	}
}
