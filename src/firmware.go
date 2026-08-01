package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/linuxboot/fiano/pkg/guid"
	"github.com/linuxboot/fiano/pkg/uefi"
	"github.com/linuxboot/fiano/pkg/vi= *guid.MustParse(
	"F74D20EE-37E7-48FC-97F7-9B1047749C69",
)

type bitmapLocation struct {
	offset int
	length int
}

type logoMatch struct {
	section  *uefi.Section
	location bitmapLocation
}

type logoFinder struct {
	files   int
	matches []logoMatch
}

func replaceBootLogo(
	imagePath string,
	firmwarePath string,
	outputPath string,
) error {
	image, err := readBitmap(imagePath)
	if err != nil {
		return err
	}

	firmware, err := readFirmware(firmwarePath)
	if err != nil {
		return err
	}

	match, err := findBootLogo(firmware)
	if err != nil {
		return err
	}

	if err := replaceEmbeddedBitmap(
		match.section,
		match.location,
		image,
	); err != nil {
		return err
	}

	assembler := &visitors.Assemble{}

	if err := assembler.Run(firmware); err != nil {
		return fmt.Errorf("assemble firmware: %w", err)
	}

	output := firmware.Buf()

	if len(output) == 0 {
		return fmt.Errorf("assembled firmware is empty")
	}

	mode, err := fileMode(firmwarePath)
	if err != nil {
		return err
	}

	if err := writeFileAtomic(outputPath, output, mode); err != nil {
		return fmt.Errorf("write firmware: %w", err)
	}

	return nil
}

func extractBootLogo(
	firmwarePath string,
	outputPath string,
) error {
	firmware, err := readFirmware(firmwarePath)
	if err != nil {
		return err
	}

	match, err := findBootLogo(firmware)
	if err != nil {
		return err
	}

	sectionData := match.section.Buf()
	start := match.location.offset
	end := start + match.location.length

	image := make([]byte, match.location.length)
	copy(image, sectionData[start:end])

	if err := writeFileAtomic(outputPath, image, 0o644); err != nil {
		return fmt.Errorf("write image: %w", err)
	}

	return nil
}

func readFirmware(path string) (uefi.Firmware, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read firmware %q: %w", path, err)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("firmware %q is empty", path)
	}

	firmware, err := uefi.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parse firmware %q: %w", path, err)
	}

	return firmware, nil
}

func findBootLogo(firmware uefi.Firmware) (logoMatch, error) {
	finder := &logoFinder{}

	if err := finder.Run(firmware); err != nil {
		return logoMatch{}, err
	}

	if finder.files == 0 {
		return logoMatch{}, fmt.Errorf(
			"LogoDxe file %s was not found",
			logoFileGUID.String(),
		)
	}

	switch len(finder.matches) {
	case 0:
		return logoMatch{}, fmt.Errorf(
			"no bitmap was found in LogoDxe",
		)

	case 1:
		return finder.matches[0], nil

	default:
		return logoMatch{}, fmt.Errorf(
			"multiple bitmaps were found in LogoDxe: %d",
			len(finder.matches),
		)
	}
}

func (finder *logoFinder) Run(firmware uefi.Firmware) error {
	return firmware.Apply(finder)
}

func (finder *logoFinder) Visit(firmware uefi.Firmware) error {
	file, ok := firmware.(*uefi.File)
	if !ok {
		return firmware.ApplyChildren(finder)
	}

	if file.Header.GUID != logoFileGUID {
		return file.ApplyChildren(finder)
	}

	finder.files++

	for _, section := range file.Sections {
		if err := finder.collectSection(section); err != nil {
			return err
		}
	}

	return nil
}

func (finder *logoFinder) collectSection(
	section *uefi.Section,
) error {
	if len(section.Encapsulated) != 0 {
		for _, child := range section.Encapsulated {
			if child == nil || child.Value == nil {
				continue
			}

			if nested, ok := child.Value.(*uefi.Section); ok {
				if err := finder.collectSection(nested); err != nil {
					return err
				}

				continue
			}

			visitor := &logoSectionVisitor{
				finder: finder,
			}

			if err := visitor.Run(child.Value); err != nil {
				return err
			}
		}

		return nil
	}

	locations, err := findEmbeddedBitmaps(section.Buf())
	if err != nil {
		return fmt.Errorf("inspect firmware section: %w", err)
	}

	for _, location := range locations {
		finder.matches = append(
			finder.matches,
			logoMatch{
				section:  section,
				location: location,
			},
		)
	}

	return nil
}

type logoSectionVisitor struct {
	finder *logoFinder
}

func (visitor *logoSectionVisitor) Run(
	firmware uefi.Firmware,
) error {
	return firmware.Apply(visitor)
}

func (visitor *logoSectionVisitor) Visit(
	firmware uefi.Firmware,
) error {
	if section, ok := firmware.(*uefi.Section); ok {
		return visitor.finder.collectSection(section)
	}

	return firmware.ApplyChildren(visitor)
}

func fileMode(path string) (os.FileMode, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("stat %q: %w", path, err)
	}

	return info.Mode().Perm(), nil
}

func writeFileAtomic(
	path string,
	data []byte,
	mode os.FileMode,
) error {
	directory := filepath.Dir(path)
	base := filepath.Base(path)

	temp, err := os.CreateTemp(
		directory,
		"."+base+".tmp-*",
	)
	if err != nil {
		return err
	}

	tempPath := temp.Name()
	keep := false

	defer func() {
		_ = temp.Close()

		if !keep {
			_ = os.Remove(tempPath)
		}
	}()

	if err := temp.Chmod(mode); err != nil {
		return err
	}

	if _, err := temp.Write(data); err != nil {
		return err
	}

	if err := temp.Sync(); err != nil {
		return err
	}

	if err := temp.Close(); err != nil {
		return err
	}

	if err := os.Rename(tempPath, path); err != nil {
		return err
	}

	keep = true

	return nil
}
