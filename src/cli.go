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
)

type options struct {
	command      command
	imagePath    string
	firmwarePath string
	outputPath   string
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
		return replaceBootLogo(
			options.imagePath,
			options.firmwarePath,
			options.outputPath,
		)

	case commandExtract:
		return extractBootLogo(
			options.firmwarePath,
			options.outputPath,
		)

	default:
		return fmt.Errorf(
			"unsupported command: %s",
			options.command,
		)
	}
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
	fmt.Fprintln(writer)
	fmt.Fprintln(writer, "Options:")
	fmt.Fprintln(
		writer,
		"  -o, --output <path>  Write to a different output path",
	)
	fmt.Fprintln(
		writer,
		"  -h, --help           Show usage information",
	)
	fmt.Fprintln(
		writer,
		"  -v, --version        Show version information",
	)
}
