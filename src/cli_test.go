package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewLogoResizeReporter(t *testing.T) {
	var output bytes.Buffer

	reporter := newLogoResizeReporter(
		&output,
		false,
	)
	if reporter == nil {
		t.Fatal("newLogoResizeReporter() returned nil")
	}

	err := reporter(logoResizeInfo{
		originalWidth:  3840,
		originalHeight: 2160,
		resizedWidth:   742,
		resizedHeight:  417,
	})
	if err != nil {
		t.Fatalf(
			"resize reporter returned an error: %v",
			err,
		)
	}

	want := "Logo resolution 3840x2160 exceeds the available firmware space. Resizing to 742x417 to fit.\n"
	if output.String() != want {
		t.Fatalf(
			"resize report = %q, want %q",
			output.String(),
			want,
		)
	}

	if quietReporter := newLogoResizeReporter(
		&output,
		true,
	); quietReporter != nil {
		t.Fatal(
			"newLogoResizeReporter() returned a reporter in quiet mode",
		)
	}
}

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

func TestParseOptionsAllowsOptionsBeforeCommand(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantCommand  command
		wantImage    string
		wantFirmware string
		wantJSON     bool
		wantQuiet    bool
	}{
		{
			name: "replace",
			args: []string{
				"--quiet",
				"replace",
				"logo.bmp",
				"firmware.fd",
			},
			wantCommand:  commandReplace,
			wantImage:    "logo.bmp",
			wantFirmware: "firmware.fd",
			wantQuiet:    true,
		},
		{
			name: "extract",
			args: []string{
				"--quiet",
				"extract",
				"firmware.fd",
			},
			wantCommand:  commandExtract,
			wantFirmware: "firmware.fd",
			wantQuiet:    true,
		},
		{
			name: "info",
			args: []string{
				"--json",
				"info",
				"firmware.fd",
			},
			wantCommand:  commandInfo,
			wantFirmware: "firmware.fd",
			wantJSON:     true,
		},
		{
			name: "verify",
			args: []string{
				"--quiet",
				"verify",
				"firmware.fd",
			},
			wantCommand:  commandVerify,
			wantFirmware: "firmware.fd",
			wantQuiet:    true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, handled, err := parseOptions(test.args)
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

			if result.command != test.wantCommand {
				t.Errorf(
					"command = %q, want %q",
					result.command,
					test.wantCommand,
				)
			}

			if result.imagePath != test.wantImage {
				t.Errorf(
					"imagePath = %q, want %q",
					result.imagePath,
					test.wantImage,
				)
			}

			if result.firmwarePath != test.wantFirmware {
				t.Errorf(
					"firmwarePath = %q, want %q",
					result.firmwarePath,
					test.wantFirmware,
				)
			}

			if result.json != test.wantJSON {
				t.Errorf(
					"json = %t, want %t",
					result.json,
					test.wantJSON,
				)
			}

			if result.quiet != test.wantQuiet {
				t.Errorf(
					"quiet = %t, want %t",
					result.quiet,
					test.wantQuiet,
				)
			}
		})
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

func TestParseOptionsRejectsUnknownCommand(
	t *testing.T,
) {
	var output bytes.Buffer

	previousStderr := stderr
	stderr = &output

	t.Cleanup(func() {
		stderr = previousStderr
	})

	_, _, err := parseOptions([]string{
		"supply",
		"logo1.ffs",
	})

	if err == nil {
		t.Fatal(
			"parseOptions() accepted an unknown command",
		)
	}

	if !strings.Contains(
		err.Error(),
		"unknown command: supply",
	) {
		t.Fatalf(
			"parseOptions() error = %q, want unknown command error",
			err,
		)
	}

	if !strings.Contains(output.String(), "Usage:") {
		t.Fatal(
			"unknown command did not print usage information",
		)
	}
}

func TestParseOptionsAllowsExplicitDotlessPath(
	t *testing.T,
) {
	result, handled, err := parseOptions([]string{
		"./supply",
		"logo1.ffs",
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

	if result.imagePath != "./supply" {
		t.Errorf(
			"imagePath = %q, want %q",
			result.imagePath,
			"./supply",
		)
	}

	if result.firmwarePath != "logo1.ffs" {
		t.Errorf(
			"firmwarePath = %q, want %q",
			result.firmwarePath,
			"logo1.ffs",
		)
	}
}

func TestParseOptionsAllowsDotlessPathAfterDoubleDash(
	t *testing.T,
) {
	result, handled, err := parseOptions([]string{
		"--",
		"supply",
		"logo1.ffs",
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

	if result.imagePath != "supply" {
		t.Errorf(
			"imagePath = %q, want %q",
			result.imagePath,
			"supply",
		)
	}

	if result.firmwarePath != "logo1.ffs" {
		t.Errorf(
			"firmwarePath = %q, want %q",
			result.firmwarePath,
			"logo1.ffs",
		)
	}
}

func TestLooksLikeCommand(t *testing.T) {
	tests := []struct {
		name     string
		argument string
		want     bool
	}{
		{name: "command", argument: "replace", want: true},
		{name: "unknown command", argument: "supply", want: true},
		{name: "file with extension", argument: "logo.bmp", want: false},
		{name: "relative file with extension", argument: "./logo.bmp", want: false},
		{name: "path without extension", argument: "./logo", want: true},
		{name: "option", argument: "--help", want: false},
		{name: "double dash", argument: "--", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := looksLikeCommand(test.argument)

			if got != test.want {
				t.Fatalf(
					"looksLikeCommand(%q) = %t, want %t",
					test.argument,
					got,
					test.want,
				)
			}
		})
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

func TestPrintSuccessReplace(t *testing.T) {
	var output bytes.Buffer

	if err := printSuccess(
		&output,
		options{
			command:    commandReplace,
			outputPath: "modified.fd",
		},
	); err != nil {
		t.Fatalf(
			"printSuccess() returned an error: %v",
			err,
		)
	}

	expected := "Boot logo replaced successfully: modified.fd\n"

	if output.String() != expected {
		t.Fatalf(
			"success output = %q, want %q",
			output.String(),
			expected,
		)
	}
}

func TestPrintSuccessExtract(t *testing.T) {
	var output bytes.Buffer

	if err := printSuccess(
		&output,
		options{
			command:    commandExtract,
			outputPath: "firmware.fd.logo.bmp",
		},
	); err != nil {
		t.Fatalf(
			"printSuccess() returned an error: %v",
			err,
		)
	}

	expected := "Boot logo extracted successfully: firmware.fd.logo.bmp\n"

	if output.String() != expected {
		t.Fatalf(
			"success output = %q, want %q",
			output.String(),
			expected,
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
