package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

type options struct {
	firmwarePath string
	imagePath    string
	outputPath   string
}

func run(args []string) error {
	options, handled, err := parseOptions(args, os.Stdout, os.Stderr)
	if err != nil {
		return err
	}

	if handled {
		return nil
	}

	return replaceBootLogo(
		options.firmwarePath,
		options.imagePath,
		options.outputPath,
	)
}

func parseOptions(
	args []string,
	stdout io.Writer,
	stderr io.Writer,
) (options, bool, error) {
	var options options

	flags := flag.NewFlagSet("boot-logo", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	var help bool
	var version bool

	flags.BoolVar(&help, "help", false, "show usage information")
	flags.BoolVar(&help, "h", false, "show usage information")
	flags.BoolVar(&version, "version", false, "show version information")
	flags.BoolVar(&version, "v", false, "show version information")

	if err := flags.Parse(args); err != nil {
		printUsage(stderr)

		return options, false, err
	}

	if help {
		printUsage(stdout)

		return options, true, nil
	}

	if version {
		fmt.Fprintln(stdout, Version)

		return options, true, nil
	}

	if flags.NArg() != 3 {
		printUsage(stderr)

		return options, false, fmt.Errorf(
			"expected firmware, image and output paths",
		)
	}

	options.firmwarePath = flags.Arg(0)
	options.imagePath = flags.Arg(1)
	options.outputPath = flags.Arg(2)

	return options, false, nil
}

func printUsage(writer io.Writer) {
	fmt.Fprintln(writer, "Usage:")
	fmt.Fprintln(
		writer,
		"  boot-logo [options] <firmware> <image> <output>",
	)
	fmt.Fprintln(writer)
	fmt.Fprintln(writer, "Options:")
	fmt.Fprintln(writer, "  -h, --help       Show usage information")
	fmt.Fprintln(writer, "  -v, --version    Show version information")
}
