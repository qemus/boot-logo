package main

import (
	"fmt"
	"os"

	"github.com/google/renameio/v2"
	"github.com/linuxboot/fiano/pkg/guid"
	"github.com/linuxboot/fiano/pkg/uefi"
	"github.com/linuxboot/fiano/pkg/visitors"
)

var logoFileGUID = *guid.MustParse(
	"F74D20EE-37E7-48FC-97F7-9B1047749C69",
)

type bitmapLocation struct {
	offset int
	length int
}

type logoMatch struct {
	section  *uefi.Section
	location hiiImageLocation
}

type logoFinder struct {
	files   int
	matches []logoMatch
}

func replaceBootLogo(
	replacementPath string,
	firmwarePath string,
	outputPath string,
) error {
	replacementData, err := os.ReadFile(replacementPath)
	if err != nil {
		return fmt.Errorf(
			"read replacement %q: %w",
			replacementPath,
			err,
		)
	}

	if replacementFile, ok := parseStandaloneFFS(
		replacementData,
	); ok {
		return replaceBootLogoFile(
			replacementFile,
			replacementData,
			firmwarePath,
			outputPath,
		)
	}

	return replaceBootLogoImage(
		replacementPath,
		firmwarePath,
		outputPath,
	)
}

func replaceBootLogoFile(
	replacementFile *uefi.File,
	replacementData []byte,
	firmwarePath string,
	outputPath string,
) error {
	if replacementFile == nil {
		return fmt.Errorf(
			"replacement FFS file is nil",
		)
	}

	if replacementFile.Header.GUID != logoFileGUID {
		return fmt.Errorf(
			"replacement FFS GUID is %s, want LogoDxe GUID %s",
			replacementFile.Header.GUID.String(),
			logoFileGUID.String(),
		)
	}

	firmware, err := readFirmware(firmwarePath)
	if err != nil {
		return err
	}

	destinationFile, err := findLogoFile(firmware)
	if err != nil {
		return err
	}

	if standaloneFile, ok := firmware.(*uefi.File); ok &&
		standaloneFile == destinationFile {
		return writeFirmwareOutput(
			replacementData,
			firmwarePath,
			outputPath,
		)
	}

	replacer := &visitors.Insert{
		Predicate: visitors.FindFileGUIDPredicate(
			logoFileGUID,
		),
		NewFile:    replacementFile,
		InsertType: visitors.InsertTypeReplaceFFS,
	}

	if err := replacer.Run(firmware); err != nil {
		return fmt.Errorf(
			"replace LogoDxe FFS file: %w",
			err,
		)
	}

	return assembleAndWriteFirmware(
		firmware,
		firmwarePath,
		outputPath,
	)
}

func replaceBootLogoImage(
	imagePath string,
	firmwarePath string,
	outputPath string,
) error {
	bitmap, err := readBitmap(imagePath)
	if err != nil {
		return err
	}

	source, err := decodeBitmap(bitmap)
	if err != nil {
		return fmt.Errorf(
			"decode converted replacement image: %w",
			err,
		)
	}

	firmware, err := readFirmware(firmwarePath)
	if err != nil {
		return err
	}

	match, err := findBootLogo(firmware)
	if err != nil {
		return err
	}

	peImage, err := sectionPayload(match.section)
	if err != nil {
		return err
	}

	updatedPE, err := replaceHIIImage(
		peImage,
		match.location,
		source,
	)
	if err != nil {
		return fmt.Errorf(
			"replace HII boot logo: %w",
			err,
		)
	}

	replacer := &visitors.ReplacePE32{
		Predicate: func(candidate uefi.Firmware) bool {
			file, ok := candidate.(*uefi.File)
			if !ok {
				return false
			}

			return file.Header.GUID == logoFileGUID
		},
		NewPE32: updatedPE,
	}

	if err := replacer.Run(firmware); err != nil {
		return fmt.Errorf(
			"replace LogoDxe PE32 section: %w",
			err,
		)
	}

	if len(replacer.Matches) == 0 {
		return fmt.Errorf(
			"LogoDxe PE32 section was not replaced",
		)
	}

	if len(replacer.Matches) > 1 {
		return fmt.Errorf(
			"multiple LogoDxe PE32 sections were replaced: %d",
			len(replacer.Matches),
		)
	}

	matchedFile, ok := replacer.Matches[0].(*uefi.File)
	if !ok {
		return fmt.Errorf(
			"matched LogoDxe object is %T, want *uefi.File",
			replacer.Matches[0],
		)
	}

	matchedFile.Modified = true

	return assembleAndWriteFirmware(
		firmware,
		firmwarePath,
		outputPath,
	)
}

func assembleAndWriteFirmware(
	firmware uefi.Firmware,
	firmwarePath string,
	outputPath string,
) error {
	assembler := &visitors.Assemble{}

	if err := assembler.Run(firmware); err != nil {
		return fmt.Errorf(
			"assemble firmware: %w",
			err,
		)
	}

	output := firmware.Buf()

	if len(output) == 0 {
		return fmt.Errorf(
			"assembled firmware is empty",
		)
	}

	return writeFirmwareOutput(
		output,
		firmwarePath,
		outputPath,
	)
}

func writeFirmwareOutput(
	output []byte,
	firmwarePath string,
	outputPath string,
) error {
	mode, err := fileMode(firmwarePath)
	if err != nil {
		return err
	}

	if err := writeFileAtomic(
		outputPath,
		output,
		mode,
	); err != nil {
		return fmt.Errorf(
			"write firmware: %w",
			err,
		)
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

	peImage, err := sectionPayload(match.section)
	if err != nil {
		return err
	}

	source, err := decodeHIIImage(
		peImage,
		match.location,
	)
	if err != nil {
		return fmt.Errorf(
			"decode HII boot logo: %w",
			err,
		)
	}

	bitmap, err := encodeBitmap(source)
	if err != nil {
		return fmt.Errorf(
			"encode extracted boot logo: %w",
			err,
		)
	}

	if err := writeFileAtomic(
		outputPath,
		bitmap,
		0o644,
	); err != nil {
		return fmt.Errorf(
			"write image: %w",
			err,
		)
	}

	return nil
}

func readFirmware(
	path string,
) (uefi.Firmware, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf(
			"read firmware %q: %w",
			path,
			err,
		)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf(
			"firmware %q is empty",
			path,
		)
	}

	if file, ok := parseStandaloneFFS(data); ok {
		return file, nil
	}

	firmware, err := uefi.Parse(data)
	if err != nil {
		return nil, fmt.Errorf(
			"parse firmware %q: %w",
			path,
			err,
		)
	}

	return firmware, nil
}

func parseStandaloneFFS(
	data []byte,
) (*uefi.File, bool) {
	if len(data) < uefi.FileHeaderMinLength {
		return nil, false
	}

	file, err := uefi.NewFile(data)
	if err != nil || file == nil {
		return nil, false
	}

	if file.Header.ExtendedSize != uint64(len(data)) {
		return nil, false
	}

	return file, true
}

func findLogoFile(
	firmware uefi.Firmware,
) (*uefi.File, error) {
	finder := &visitors.Find{
		Predicate: visitors.FindFileGUIDPredicate(
			logoFileGUID,
		),
	}

	if err := finder.Run(firmware); err != nil {
		return nil, fmt.Errorf(
			"find LogoDxe file: %w",
			err,
		)
	}

	switch len(finder.Matches) {
	case 0:
		return nil, fmt.Errorf(
			"LogoDxe file %s was not found",
			logoFileGUID.String(),
		)

	case 1:
		file, ok := finder.Matches[0].(*uefi.File)
		if !ok {
			return nil, fmt.Errorf(
				"matched LogoDxe object is %T, want *uefi.File",
				finder.Matches[0],
			)
		}

		return file, nil

	default:
		return nil, fmt.Errorf(
			"multiple LogoDxe files were found: %d",
			len(finder.Matches),
		)
	}
}

func findBootLogo(
	firmware uefi.Firmware,
) (logoMatch, error) {
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
			"no HII boot logo was found in LogoDxe",
		)

	case 1:
		return finder.matches[0], nil

	default:
		return logoMatch{}, fmt.Errorf(
			"multiple HII boot logos were found in LogoDxe: %d",
			len(finder.matches),
		)
	}
}

func (finder *logoFinder) Run(
	firmware uefi.Firmware,
) error {
	return firmware.Apply(finder)
}

func (finder *logoFinder) Visit(
	firmware uefi.Firmware,
) error {
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
	if section == nil {
		return nil
	}

	if section.Header.Type == uefi.SectionTypePE32 {
		peImage, err := sectionPayload(section)
		if err != nil {
			return err
		}

		images, err := findHIIImages(peImage)
		if err != nil {
			return fmt.Errorf(
				"inspect LogoDxe PE32 section: %w",
				err,
			)
		}

		for _, location := range images {
			finder.matches = append(
				finder.matches,
				logoMatch{
					section:  section,
					location: location,
				},
			)
		}
	}

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

func sectionPayload(
	section *uefi.Section,
) ([]byte, error) {
	if section == nil {
		return nil, fmt.Errorf(
			"firmware section is nil",
		)
	}

	data := section.Buf()

	headerSize := uefi.SectionMinLength

	if section.Header.Size == [3]uint8{
		0xff,
		0xff,
		0xff,
	} {
		headerSize = uefi.SectionExtMinLength
	}

	if len(data) < headerSize {
		return nil, fmt.Errorf(
			"firmware section header is incomplete",
		)
	}

	return data[headerSize:], nil
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

func fileMode(
	path string,
) (os.FileMode, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf(
			"stat %q: %w",
			path,
			err,
		)
	}

	return info.Mode().Perm(), nil
}

func writeFileAtomic(
	path string,
	data []byte,
	mode os.FileMode,
) error {
	return renameio.WriteFile(
		path,
		data,
		mode,
		renameio.IgnoreUmask(),
	)
}
