package main

import (
	"fmt"

	"github.com/linuxboot/fiano/pkg/compression"
	"github.com/linuxboot/fiano/pkg/uefi"
	"github.com/linuxboot/fiano/pkg/visitors"
)

type bootLogoLayout struct {
	volume   *uefi.FirmwareVolume
	file     *uefi.File
	sections []*uefi.Section
}

func findBootLogoLayout(
	firmware uefi.Firmware,
	targetSection *uefi.Section,
) (bootLogoLayout, error) {
	file, err := findLogoFile(firmware)
	if err != nil {
		return bootLogoLayout{}, err
	}

	sections, found := findBootLogoSectionPath(
		file.Sections,
		targetSection,
	)
	if !found {
		return bootLogoLayout{}, fmt.Errorf(
			"LogoDxe PE32 section path was not found",
		)
	}

	volume, err := findContainingFirmwareVolume(
		firmware,
		file,
	)
	if err != nil {
		return bootLogoLayout{}, err
	}

	return bootLogoLayout{
		volume:   volume,
		file:     file,
		sections: sections,
	}, nil
}

func findBootLogoSectionPath(
	sections []*uefi.Section,
	target *uefi.Section,
) ([]*uefi.Section, bool) {
	for _, section := range sections {
		if section == nil {
			continue
		}

		if section == target {
			return []*uefi.Section{section}, true
		}

		var nested []*uefi.Section
		for _, child := range section.Encapsulated {
			if child == nil || child.Value == nil {
				continue
			}

			childSection, ok := child.Value.(*uefi.Section)
			if !ok {
				continue
			}

			nested = append(
				nested,
				childSection,
			)
		}

		path, found := findBootLogoSectionPath(
			nested,
			target,
		)
		if found {
			return append(
				[]*uefi.Section{section},
				path...,
			), true
		}
	}

	return nil, false
}

func findContainingFirmwareVolume(
	firmware uefi.Firmware,
	file *uefi.File,
) (*uefi.FirmwareVolume, error) {
	finder := &visitors.Find{
		Predicate: func(candidate uefi.Firmware) bool {
			_, ok := candidate.(*uefi.FirmwareVolume)
			return ok
		},
	}

	if err := finder.Run(firmware); err != nil {
		return nil, fmt.Errorf(
			"find containing firmware volume: %w",
			err,
		)
	}

	var containing []*uefi.FirmwareVolume

	for _, match := range finder.Matches {
		volume, ok := match.(*uefi.FirmwareVolume)
		if !ok {
			continue
		}

		for _, candidate := range volume.Files {
			if candidate == file {
				containing = append(
					containing,
					volume,
				)
				break
			}
		}
	}

	switch len(containing) {
	case 0:
		if standalone, ok := firmware.(*uefi.File); ok &&
			standalone == file {
			return nil, nil
		}

		return nil, fmt.Errorf(
			"firmware volume containing LogoDxe was not found",
		)

	case 1:
		return containing[0], nil

	default:
		return nil, fmt.Errorf(
			"multiple firmware volumes contain LogoDxe: %d",
			len(containing),
		)
	}
}

func bootLogoPathUsesCompression(
	sections []*uefi.Section,
) bool {
	for _, section := range sections {
		if section == nil ||
			section.Header.Type != uefi.SectionTypeGUIDDefined ||
			section.TypeSpecific == nil ||
			section.TypeSpecific.Header == nil {
			continue
		}

		defined, ok := section.TypeSpecific.Header.(*uefi.SectionGUIDDefined)
		if ok && defined.Attributes&uint16(
			uefi.GUIDEDSectionProcessingRequired,
		) != 0 {
			return true
		}
	}

	return false
}

func bootLogoLayoutFits(
	layout bootLogoLayout,
	updatedPE []byte,
) (bool, error) {
	plannedFile, err := planLogoFile(
		layout.file,
		layout.sections,
		updatedPE,
	)
	if err != nil {
		return false, err
	}

	if layout.volume == nil || layout.volume.Resizable {
		return true, nil
	}

	end, err := plannedFirmwareVolumeEnd(
		layout.volume,
		layout.file,
		plannedFile,
	)
	if err != nil {
		return false, err
	}

	return end <= layout.volume.Length, nil
}

func planLogoFile(
	file *uefi.File,
	sections []*uefi.Section,
	updatedPE []byte,
) (*uefi.File, error) {
	if file == nil || len(sections) == 0 {
		return nil, fmt.Errorf(
			"LogoDxe layout is incomplete",
		)
	}

	updatedSection, err := planLogoSection(
		sections,
		0,
		updatedPE,
	)
	if err != nil {
		return nil, err
	}

	fileData := make([]byte, 0, len(file.Buf()))
	dataLength := uint64(0)
	found := false

	for _, section := range file.Sections {
		for count := uefi.Align4(dataLength) - dataLength; count > 0; count-- {
			fileData = append(fileData, 0)
		}
		dataLength = uefi.Align4(dataLength)

		sectionData := section.Buf()
		if section == sections[0] {
			sectionData = updatedSection
			found = true
		}

		fileData = append(fileData, sectionData...)
		dataLength += uint64(len(sectionData))
	}

	if !found {
		return nil, fmt.Errorf(
			"top-level LogoDxe section was not found",
		)
	}

	planned := *file
	planned.Sections = nil
	planned.NVarStore = nil
	planned.SetSize(
		uefi.FileHeaderMinLength+dataLength,
		true,
	)

	if err := planned.ChecksumAndAssemble(fileData); err != nil {
		return nil, fmt.Errorf(
			"plan LogoDxe FFS file: %w",
			err,
		)
	}

	return &planned, nil
}

func planLogoSection(
	sections []*uefi.Section,
	index int,
	updatedPE []byte,
) ([]byte, error) {
	section := sections[index]
	planned, err := cloneSectionForPlan(section)
	if err != nil {
		return nil, err
	}

	if index == len(sections)-1 {
		planned.SetBuf(updatedPE)
		planned.Encapsulated = nil

		if err := planned.GenSecHeader(); err != nil {
			return nil, fmt.Errorf(
				"plan LogoDxe PE32 section: %w",
				err,
			)
		}

		return planned.Buf(), nil
	}

	next := sections[index+1]
	sectionData := make([]byte, 0, len(section.Buf()))
	dataLength := uint64(0)
	found := false

	for _, child := range section.Encapsulated {
		if child == nil || child.Value == nil {
			continue
		}

		for count := uefi.Align4(dataLength) - dataLength; count > 0; count-- {
			sectionData = append(sectionData, 0)
		}
		dataLength = uefi.Align4(dataLength)

		childData := child.Value.Buf()
		if childSection, ok := child.Value.(*uefi.Section); ok &&
			childSection == next {
			childData, err = planLogoSection(
				sections,
				index+1,
				updatedPE,
			)
			if err != nil {
				return nil, err
			}
			found = true
		}

		sectionData = append(sectionData, childData...)
		dataLength += uint64(len(childData))
	}

	if !found {
		return nil, fmt.Errorf(
			"nested LogoDxe section was not found",
		)
	}

	if section.Header.Type == uefi.SectionTypeGUIDDefined &&
		section.TypeSpecific != nil &&
		section.TypeSpecific.Header != nil {
		defined, ok := section.TypeSpecific.Header.(*uefi.SectionGUIDDefined)
		if ok && defined.Attributes&uint16(
			uefi.GUIDEDSectionProcessingRequired,
		) != 0 {
			encoder := compression.CompressorFromGUID(
				&defined.GUID,
			)
			if encoder == nil {
				return nil, fmt.Errorf(
					"unsupported LogoDxe section compression GUID %s",
					defined.GUID.String(),
				)
			}

			sectionData, err = encoder.Encode(sectionData)
			if err != nil {
				return nil, fmt.Errorf(
					"compress planned LogoDxe section: %w",
					err,
				)
			}
		}
	}

	planned.SetBuf(sectionData)
	planned.Encapsulated = nil

	if err := planned.GenSecHeader(); err != nil {
		return nil, fmt.Errorf(
			"plan encapsulating LogoDxe section: %w",
			err,
		)
	}

	return planned.Buf(), nil
}

func cloneSectionForPlan(
	section *uefi.Section,
) (*uefi.Section, error) {
	if section == nil {
		return nil, fmt.Errorf(
			"firmware section is nil",
		)
	}

	planned := *section

	if section.TypeSpecific != nil {
		typeSpecific := *section.TypeSpecific

		if defined, ok := section.TypeSpecific.Header.(*uefi.SectionGUIDDefined); ok {
			definedCopy := *defined
			typeSpecific.Header = &definedCopy
		}

		planned.TypeSpecific = &typeSpecific
	}

	return &planned, nil
}

func plannedFirmwareVolumeEnd(
	volume *uefi.FirmwareVolume,
	logoFile *uefi.File,
	plannedLogoFile *uefi.File,
) (uint64, error) {
	fileOffset := volume.DataOffset
	found := false

	for _, file := range volume.Files {
		if file == nil || file.Header.Type == uefi.FVFileTypePad {
			continue
		}

		candidate := file
		if file == logoFile {
			candidate = plannedLogoFile
			found = true
		}

		fileLength := uint64(len(candidate.Buf()))
		if fileLength == 0 {
			return 0, fmt.Errorf(
				"firmware file %s is empty",
				candidate.Header.GUID.String(),
			)
		}

		alignedOffset := uefi.Align8(fileOffset)
		if alignment := candidate.Header.Attributes.GetAlignment(); alignment != 1 {
			headerLength := candidate.HeaderLen()
			fileDataOffset := uefi.Align(
				alignedOffset+headerLength,
				alignment,
			)
			newOffset := fileDataOffset - headerLength

			if gap := newOffset - alignedOffset; gap >= 8 && gap < uefi.FileHeaderMinLength {
				fileDataOffset = uefi.Align(
					fileDataOffset+1,
					alignment,
				)
				newOffset = fileDataOffset - headerLength
			}

			alignedOffset = newOffset
		}

		fileOffset = alignedOffset + fileLength
	}

	if !found {
		return 0, fmt.Errorf(
			"LogoDxe file was not found in its firmware volume",
		)
	}

	return fileOffset, nil
}
