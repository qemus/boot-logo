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
		t.Fatalf(
			"parseOptions() returned an error: %v",
			err,
		)
	}

	if handled {
		t.Fatal(
			"parseOptions() unexpectedly handled the command",
		)
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

	if result.outputPath != "firmware.fd" {
		t.Errorf(
			"outputPath = %q, want %q",
			result.outputPath,
			"firmware.fd",
		)
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
		t.Fatalf(
			"parseOptions() returned an error: %v",
			err,
		)
	}

	if handled {
		t.Fatal(
			"parseOptions() unexpectedly handled the command",
		)
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
		t.Fatalf(
			"parseOptions() returned an error: %v",
			err,
		)
	}

	if handled {
		t.Fatal(
			"parseOptions() unexpectedly handled the command",
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

func TestParseOptionsAllowsExplicitInputOutput(t *testing.T) {
	firmwarePath := filepath.Join(
		t.TempDir(),
		"firmware.fd",
	)

	result, handled, err := parseOptions([]string{
		"--output",
		firmwarePath,
		"logo.bmp",
		firmwarePath,
	})
	if err != nil {
		t.Fatalf(
			"parseOptions() returned an error: %v",
			err,
		)
	}

	if handled {
		t.Fatal(
			"parseOptions() unexpectedly handled the command",
		)
	}

	if result.outputPath != firmwarePath {
		t.Errorf(
			"outputPath = %q, want %q",
			result.outputPath,
			firmwarePath,
		)
	}
}

func TestParseOptionsExtract(t *testing.T) {
	result, handled, err := parseOptions([]string{
		"extract",
		"firmware.fd",
	})
	if err != nil {
		t.Fatalf(
			"parseOptions() returned an error: %v",
			err,
		)
	}

	if handled {
		t.Fatal(
			"parseOptions() unexpectedly handled the command",
		)
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
		t.Fatalf(
			"parseOptions() returned an error: %v",
			err,
		)
	}

	if handled {
		t.Fatal(
			"parseOptions() unexpectedly handled the command",
		)
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
		t.Fatalf(
			"parseOptions() returned an error: %v",
			err,
		)
	}

	if handled {
		t.Fatal(
			"parseOptions() unexpectedly handled the command",
		)
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

	if result.outputPath != "-firmware.fd" {
		t.Errorf(
			"outputPath = %q, want %q",
			result.outputPath,
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

	_, handled, err := parseOptions([]string{
		"--help",
	})
	if err != nil {
		t.Fatalf(
			"parseOptions() returned an error: %v",
			err,
		)
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

	if !strings.Contains(output.String(), "in-place") {
		t.Fatalf(
			"help output does not describe default in-place replacement:\n%s",
			output.String(),
		)
	}

	if strings.Contains(output.String(), "--in-place") {
		t.Fatalf(
			"help output still documents --in-place:\n%s",
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

	_, handled, err := parseOptions([]string{
		"--version",
	})
	if err != nil {
		t.Fatalf(
			"parseOptions() returned an error: %v",
			err,
		)
	}

	if !handled {
		t.Fatal(
			"parseOptions() did not handle --version",
		)
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
		t.Fatal(
			"parseOptions() accepted an unknown option",
		)
	}
}

func TestParseOptionsRejectsRemovedInPlaceOption(
	t *testing.T,
) {
	_, _, err := parseOptions([]string{
		"--in-place",
		"logo.bmp",
		"firmware.fd",
	})

	if err == nil {
		t.Fatal(
			"parseOptions() accepted the removed --in-place option",
		)
	}

	if !strings.Contains(err.Error(), "unknown option") {
		t.Fatalf(
			"parseOptions() error = %q, want unknown option error",
			err,
		)
	}
}

func TestParseOptionsRejectsMissingOutputPath(
	t *testing.T,
) {
	_, _, err := parseOptions([]string{
		"--output",
	})

	if err == nil {
		t.Fatal(
			"parseOptions() accepted --output without a path",
		)
	}
}

func TestParseOptionsRejectsEmptyOutputPath(
	t *testing.T,
) {
	_, _, err := parseOptions([]string{
		"--output=",
		"logo.bmp",
		"firmware.fd",
	})

	if err == nil {
		t.Fatal(
			"parseOptions() accepted an empty output path",
		)
	}
}

func TestParseOptionsRejectsExtractOverwrite(
	t *testing.T,
) {
	firmwarePath := filepath.Join(
		t.TempDir(),
		"firmware.fd",
	)

	_, _, err := parseOptions([]string{
		"extract",
		"--output",
		firmwarePath,
		firmwarePath,
	})

	if err == nil {
		t.Fatal(
			"parseOptions() allowed extraction over the firmware",
		)
	}

	if !strings.Contains(
		err.Error(),
		"cannot overwrite",
	) {
		t.Fatalf(
			"parseOptions() error = %q, want overwrite error",
			err,
		)
	}
}

func TestParseOptionsRejectsMissingReplaceArguments(
	t *testing.T,
) {
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
		t.Fatal(
			"parseOptions() accepted missing replace arguments",
		)
	}

	if !strings.Contains(output.String(), "Usage:") {
		t.Fatal(
			"missing arguments did not print usage information",
		)
	}
}

func TestParseOptionsRejectsMissingExtractArgument(
	t *testing.T,
) {
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
		t.Fatal(
			"parseOptions() accepted a missing firmware path",
		)
	}

	if !strings.Contains(output.String(), "Usage:") {
		t.Fatal(
			"missing firmware path did not print usage information",
		)
	}
}

func TestSamePath(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(
		directory,
		"firmware.fd",
	)

	if !samePath(path, filepath.Clean(path)) {
		t.Fatal(
			"samePath() returned false for identical paths",
		)
	}

	if samePath(
		path,
		filepath.Join(directory, "other.fd"),
	) {
		t.Fatal(
			"samePath() returned true for different paths",
		)
	}
}
