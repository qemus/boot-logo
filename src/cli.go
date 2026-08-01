package main

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

type command string

const (
	commandReplace command = "replace"
	commandExtract command = "extract"
	commandInfo    command = "info"
	commandVerify  command = "verify"
)

type options struct {
	command      command
	imagePath    string
	firmwarePath string
	outputPath   string
	json         bool
	quiet        bool
}

func run(args []string) error {
	options, handled, err := parseOptions(args)
	if err != nil {
		return err
	}

	if handled {
		return nil
	}

	switch options.command {
	case commandReplace:
		err = replaceBootLogo(
			options.imagePath,
			options.firmwarePath,
			options.outputPath,
		)

	case commandExtract:
		err = extractBootLogo(
			options.firmwarePath,
			options.outputPath,
		)

	case commandInfo:
		var info firmwareInfo

		info, err = inspectFirmware(options.firmwarePath)
		if err == nil {
			err = printFirmwareInfo(
				stdout,
				info,
				options.json,
			)
		}

	case commandVerify:
		err = verifyFirmware(options.firmwarePath)

	default:
		return fmt.Errorf(
			"unsupported command: %s",
			options.command,
		)
	}

	if err != nil {
		return err
	}

	printSuccess(stdout, options)

	return nil
}

func parseOptions(args []string) (options, bool, error) {
	options := options{
		command: commandReplace,
	}

	if len(args) > 0 {
		switch args[0] {
		case string(commandReplace):
			args = args[1:]

		case string(commandExtract):
			options.command = commandExtract
			args = args[1:]

		case string(commandInfo):
			options.command = commandInfo
			args = args[1:]

		case string(commandVerify):
			options.command = commandVerify
			args = args[1:]
		}
	}

	var positional []string

	for index := 0; index < len(args); index++ {
		argument := args[index]

		switch {
		case argument == "--":
			positional = append(
				positional,
				args[index+1:]...,
			)

			index = len(args)

		case argument == "-h" || argument == "--help":
			printUsage(stdout)

			return options, true, nil

		case argument == "-v" || argument == "--version":
			fmt.Fprintln(stdout, Version)

			return options, true, nil

		case argument == "-o" || argument == "--output":
			index++

			if index >= len(args) {
				return options, false, fmt.Errorf(
					"%s requires a path",
					argument,
				)
			}

			options.outputPath = args[index]

		case strings.HasPrefix(argument, "--output="):
			options.outputPath = strings.TrimPrefix(
				argument,
				"--output=",
			)

			if options.outputPath == "" {
				return options, false, fmt.Errorf(
					"--output requires a path",
				)
			}

		case argument == "--json":
			options.json = true

		case argument == "-q" || argument == "--quiet":
			options.quiet = true

		case strings.HasPrefix(argument, "-"):
			return options, false, fmt.Errorf(
				"unknown option: %s",
				argument,
			)

		default:
			positional = append(
				positional,
				argument,
			)
		}
	}

	if options.json && options.command != commandInfo {
		return options, false, fmt.Errorf(
			"--json is only supported by the info command",
		)
	}

	if options.quiet && options.command != commandVerify {
		return options, false, fmt.Errorf(
			"--quiet is only supported by the verify command",
		)
	}

	if options.outputPath != "" &&
		options.command != commandReplace &&
		options.command != commandExtract {
		return options, false, fmt.Errorf(
			"--output is only supported by replace and extract",
		)
	}

	switch options.command {
	case commandReplace:
		if len(positional) != 2 {
			printUsage(stderr)

			return options, false, fmt.Errorf(
				"replace requires an image and firmware path",
			)
		}

		options.imagePath = positional[0]
		options.firmwarePath = positional[1]

		if options.outputPath == "" {
			options.outputPath = options.firmwarePath
		}

	case commandExtract:
		if len(positional) != 1 {
			printUsage(stderr)

			return options, false, fmt.Errorf(
				"extract requires a firmware path",
			)
		}

		options.firmwarePath = positional[0]

		if options.outputPath == "" {
			options.outputPath =
				options.firmwarePath + ".logo.bmp"
		}

		if samePath(
			options.firmwarePath,
			options.outputPath,
		) {
			return options, false, fmt.Errorf(
				"the extracted image cannot overwrite the firmware",
			)
		}

	case commandInfo:
		if len(positional) != 1 {
			printUsage(stderr)

			return options, false, fmt.Errorf(
				"info requires a firmware path",
			)
		}

		options.firmwarePath = positional[0]

	case commandVerify:
		if len(positional) != 1 {
			printUsage(stderr)

			return options, false, fmt.Errorf(
				"verify requires a firmware path",
			)
		}

		options.firmwarePath = positional[0]
	}

	return options, false, nil
}

func samePath(first string, second string) bool {
	firstPath, firstErr := filepath.Abs(first)
	secondPath, secondErr := filepath.Abs(second)

	if firstErr != nil || secondErr != nil {
		return filepath.Clean(first) ==
			filepath.Clean(second)
	}

	return filepath.Clean(firstPath) ==
		filepath.Clean(secondPath)
}

func printSuccess(writer io.Writer, options options) {
	switch options.command {
	case commandReplace:
		fmt.Fprintf(
			writer,
			"Boot logo replaced successfully: %s\n",
			options.outputPath,
		)

	case commandExtract:
		fmt.Fprintf(
			writer,
			"Boot logo extracted successfully: %s\n",
			options.outputPath,
		)

	case commandVerify:
		if !options.quiet {
			fmt.Fprintf(
				writer,
				"Firmware verified successfully: %s\n",
				options.firmwarePath,
			)
		}
	}
}

func printUsage(writer io.Writer) {
	fmt.Fprintln(writer, "Usage:")
	fmt.Fprintln(
		writer,
		"  boot-logo [options] [replace] <image> <firmware>",
	)
	fmt.Fprintln(
		writer,
		"  boot-logo [options] extract <firmware>",
	)
	fmt.Fprintln(
		writer,
		"  boot-logo [options] info <firmware>",
	)
	fmt.Fprintln(
		writer,
		"  boot-logo [options] verify <firmware>",
	)
	fmt.Fprintln(writer)
	fmt.Fprintln(writer, "Commands:")
	fmt.Fprintln(
		writer,
		"  replace    Replace the firmware boot logo in-place (default)",
	)
	fmt.Fprintln(
		writer,
		"  extract    Extract the current firmware boot logo",
	)
	fmt.Fprintln(
		writer,
		"  info       Show firmware and embedded boot logo information",
	)
	fmt.Fprintln(
		writer,
		"  verify     Verify that the firmware and boot logo are supported",
	)
	fmt.Fprintln(writer)
	fmt.Fprintln(writer, "Options:")
	fmt.Fprintln(
		writer,
		"  -o, --output <path>  Write to a different output path",
	)
	fmt.Fprintln(
		writer,
		"      --json           Print info as JSON",
	)
	fmt.Fprintln(
		writer,
		"  -q, --quiet          Suppress successful verify output",
	)
	fmt.Fprintln(
		writer,
		"  -h, --help           Show usage information",
	)
	fmt.Fprintln(
		writer,
		"  -v, --version        Show version information",
	)
	fmt.Fprintln(writer)
}
