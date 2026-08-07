package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/linuxboot/fiano/pkg/guid"
	"github.com/linuxboot/fiano/pkg/uefi"
	"github.com/linuxboot/fiano/pkg/visitors"
)

type fixtureImageFormat struct {
	name      string
	extension string
	encode    func(io.Writer, image.Image) error
	decode    func(io.Reader) (image.Image, error)
}

var fixtureImageFormats = []fixtureImageFormat{
	{
		name:      "BMP",
		extension: ".bmp",
		encode: func(writer io.Writer, source image.Image) error {
			data, err := encodeBitmap(source)
			if err != nil {
				return err
			}
			_, err = writer.Write(data)
			return err
		},
		decode: func(reader io.Reader) (image.Image, error) {
			data, err := io.ReadAll(reader)
			if err != nil {
				return nil, err
			}
			return decodeBitmap(data)
		},
	},
	{
		name:      "PNG",
		extension: ".png",
		encode:    png.Encode,
		decode:    png.Decode,
	},
	{
		name:      "JPEG",
		extension: ".jpg",
		encode: func(writer io.Writer, source image.Image) error {
			return jpeg.Encode(writer, source, &jpeg.Options{Quality: 95})
		},
		decode: jpeg.Decode,
	},
}

var firmwareFixtures = []struct {
	name string
	file string
}{
	{name: "standalone FFS", file: "test.ffs"},
	{name: "complete ROM", file: "test.rom"},
}

func TestReadFirmwareFixtures(t *testing.T) {
	for _, fixture := range firmwareFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			path := fixturePath(fixture.file)
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("fixture %q is unavailable: %v", path, err)
			}
			firmware, err := readFirmware(path)
			if err != nil {
				t.Fatalf("readFirmware() returned an error: %v", err)
			}
			if firmware == nil {
				t.Fatal("readFirmware() returned nil")
			}
		})
	}
}

func TestFindBootLogoFixtures(t *testing.T) {
	for _, fixture := range firmwareFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			firmware, err := readFirmware(fixturePath(fixture.file))
			if err != nil {
				t.Fatalf("readFirmware() returned an error: %v", err)
			}
			match, err := findBootLogo(firmware)
			if err != nil {
				t.Fatalf("findBootLogo() returned an error: %v", err)
			}
			if match.section == nil {
				t.Fatal("findBootLogo() returned a nil section")
			}
			if match.section.Header.Type != uefi.SectionTypePE32 {
				t.Fatalf("section type = %s, want %s", match.section.Header.Type.String(), uefi.SectionTypePE32.String())
			}
			payload, err := sectionPayload(match.section)
			if err != nil {
				t.Fatalf("sectionPayload() returned an error: %v", err)
			}
			logo, err := decodeHIIImage(payload, match.location)
			if err != nil {
				t.Fatalf("decodeHIIImage() returned an error: %v", err)
			}
			bounds := logo.Bounds()
			if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
				t.Fatalf("logo dimensions = %dx%d, want positive dimensions", bounds.Dx(), bounds.Dy())
			}
		})
	}
}

func TestExtractBootLogoFixtures(t *testing.T) {
	for _, fixture := range firmwareFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "logo.bmp")
			if err := extractBootLogo(fixturePath(fixture.file), output); err != nil {
				t.Fatalf("extractBootLogo() returned an error: %v", err)
			}
			data, err := os.ReadFile(output)
			if err != nil {
				t.Fatalf("ReadFile() returned an error: %v", err)
			}
			if len(data) < len(bitmapSignature) || !bytes.Equal(data[:len(bitmapSignature)], bitmapSignature) {
				t.Fatal("extracted image does not have a BMP signature")
			}
			logo, err := decodeBitmap(data)
			if err != nil {
				t.Fatalf("decodeBitmap() returned an error: %v", err)
			}
			bounds := logo.Bounds()
			if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
				t.Fatalf("extracted dimensions = %dx%d, want positive dimensions", bounds.Dx(), bounds.Dy())
			}
		})
	}
}

func TestExtractBootLogoFixturesMatchesHIIParser(t *testing.T) {
	for _, fixture := range firmwareFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			firmware, err := readFirmware(fixturePath(fixture.file))
			if err != nil {
				t.Fatalf("readFirmware() returned an error: %v", err)
			}
			match, err := findBootLogo(firmware)
			if err != nil {
				t.Fatalf("findBootLogo() returned an error: %v", err)
			}
			payload, err := sectionPayload(match.section)
			if err != nil {
				t.Fatalf("sectionPayload() returned an error: %v", err)
			}
			expected, err := decodeHIIImage(payload, match.location)
			if err != nil {
				t.Fatalf("decodeHIIImage() returned an error: %v", err)
			}
			output := filepath.Join(t.TempDir(), "logo.bmp")
			if err := extractBootLogo(fixturePath(fixture.file), output); err != nil {
				t.Fatalf("extractBootLogo() returned an error: %v", err)
			}
			data, err := os.ReadFile(output)
			if err != nil {
				t.Fatalf("ReadFile() returned an error: %v", err)
			}
			actual, err := decodeBitmap(data)
			if err != nil {
				t.Fatalf("decodeBitmap() returned an error: %v", err)
			}
			assertFixtureImagesEqual(t, actual, expected)
		})
	}
}

func TestExtractBootLogoFixturesMatchReference(t *testing.T) {
	expected := readFixture(t, "test.bmp")
	for _, fixture := range firmwareFixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			outputPath := filepath.Join(t.TempDir(), "extracted.bmp")
			if err := extractBootLogo(fixturePath(fixture.file), outputPath); err != nil {
				t.Fatalf("extractBootLogo() returned an error: %v", err)
			}
			actual, err := os.ReadFile(outputPath)
			if err != nil {
				t.Fatalf("read extracted bitmap: %v", err)
			}
			if !bytes.Equal(actual, expected) {
				t.Fatal("extracted bitmap does not match tests/test.bmp")
			}
		})
	}
}

func TestReplaceBootLogoFixturesInPlace(t *testing.T) {
	for _, fixture := range firmwareFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			directory := t.TempDir()
			firmwarePath := filepath.Join(directory, fixture.file)
			original := copyFixture(t, fixture.file, firmwarePath, 0o640)
			replacement := fixtureReplacementImage(7, 5)
			imagePath := filepath.Join(directory, "replacement.bmp")
			writeFixtureBitmap(t, imagePath, replacement)
			if err := replaceBootLogo(imagePath, firmwarePath, firmwarePath); err != nil {
				t.Fatalf("replaceBootLogo() returned an error: %v", err)
			}
			updated, err := os.ReadFile(firmwarePath)
			if err != nil {
				t.Fatalf("ReadFile() returned an error: %v", err)
			}
			if bytes.Equal(updated, original) {
				t.Fatal("replacement did not modify the firmware")
			}
			info, err := os.Stat(firmwarePath)
			if err != nil {
				t.Fatalf("Stat() returned an error: %v", err)
			}
			if info.Mode().Perm() != 0o640 {
				t.Fatalf("firmware mode = %#o, want %#o", info.Mode().Perm(), os.FileMode(0o640))
			}
			assertFixtureReplacement(t, firmwarePath, replacement)
		})
	}
}

func TestReplaceBootLogoFixturesSeparateOutput(t *testing.T) {
	for _, fixture := range firmwareFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			directory := t.TempDir()
			inputPath := filepath.Join(directory, "input-"+fixture.file)
			original := copyFixture(t, fixture.file, inputPath, 0o644)
			outputPath := filepath.Join(directory, "output-"+fixture.file)
			replacement := fixtureReplacementImage(9, 6)
			imagePath := filepath.Join(directory, "replacement.bmp")
			writeFixtureBitmap(t, imagePath, replacement)
			if err := replaceBootLogo(imagePath, inputPath, outputPath); err != nil {
				t.Fatalf("replaceBootLogo() returned an error: %v", err)
			}
			inputAfter, err := os.ReadFile(inputPath)
			if err != nil {
				t.Fatalf("ReadFile() returned an error: %v", err)
			}
			if !bytes.Equal(inputAfter, original) {
				t.Fatal("replacement modified the input while using a separate output")
			}
			output, err := os.ReadFile(outputPath)
			if err != nil {
				t.Fatalf("ReadFile() returned an error: %v", err)
			}
			if bytes.Equal(output, original) {
				t.Fatal("separate output is unchanged")
			}
			assertFixtureReplacement(t, outputPath, replacement)
		})
	}
}

func TestReplaceBootLogoOnlyUpdatesMatchedPE32Section(t *testing.T) {
	directory := t.TempDir()
	inputPath := filepath.Join(
		directory,
		"input.ffs",
	)
	outputPath := filepath.Join(
		directory,
		"output.ffs",
	)
	imagePath := filepath.Join(
		directory,
		"replacement.bmp",
	)

	extraPE := createLogoFileWithExtraPE32(
		t,
		inputPath,
	)
	replacement := fixtureReplacementImage(
		7,
		5,
	)
	writeFixtureBitmap(
		t,
		imagePath,
		replacement,
	)

	if err := replaceBootLogoImage(
		imagePath,
		inputPath,
		outputPath,
	); err != nil {
		t.Fatalf(
			"replaceBootLogoImage() returned an error: %v",
			err,
		)
	}

	firmware, err := readFirmware(outputPath)
	if err != nil {
		t.Fatalf(
			"read updated firmware: %v",
			err,
		)
	}

	file, ok := firmware.(*uefi.File)
	if !ok {
		t.Fatalf(
			"updated firmware is %T, want *uefi.File",
			firmware,
		)
	}

	if len(file.Sections) == 0 {
		t.Fatal(
			"updated LogoDxe contains no sections",
		)
	}

	extraSection := file.Sections[len(file.Sections)-1]
	if extraSection.Header.Type != uefi.SectionTypePE32 {
		t.Fatalf(
			"extra section type = %s, want %s",
			extraSection.Header.Type.String(),
			uefi.SectionTypePE32.String(),
		)
	}

	updatedExtraPE, err := sectionPayload(extraSection)
	if err != nil {
		t.Fatalf(
			"read extra PE32 section: %v",
			err,
		)
	}

	if !bytes.Equal(updatedExtraPE, extraPE) {
		t.Fatal(
			"replacement modified an unrelated PE32 section in LogoDxe",
		)
	}

	assertFixtureReplacement(
		t,
		outputPath,
		replacement,
	)
}

func TestReplaceBootLogoStandardROM(t *testing.T) {
	originalPE := readBootLogoPE(
		t,
		fixturePath("standard.rom"),
	)

	outputPath := filepath.Join(
		t.TempDir(),
		"standard.rom",
	)

	if err := replaceBootLogo(
		fixturePath("test.bmp"),
		fixturePath("standard.rom"),
		outputPath,
	); err != nil {
		t.Fatalf(
			"replaceBootLogo() returned an error: %v",
			err,
		)
	}

	original := readFixture(t, "standard.rom")
	updated, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf(
			"read updated standard ROM: %v",
			err,
		)
	}

	if bytes.Equal(updated, original) {
		t.Fatal(
			"replacement did not modify standard.rom",
		)
	}

	replacementData := readFixture(t, "test.bmp")
	replacement, err := decodeBitmap(replacementData)
	if err != nil {
		t.Fatalf(
			"decode tests/test.bmp: %v",
			err,
		)
	}

	firmware, err := readFirmware(outputPath)
	if err != nil {
		t.Fatalf(
			"updated standard ROM cannot be parsed: %v",
			err,
		)
	}

	match, err := findBootLogo(firmware)
	if err != nil {
		t.Fatalf(
			"updated standard ROM does not contain a logo: %v",
			err,
		)
	}

	payload, err := sectionPayload(match.section)
	if err != nil {
		t.Fatalf(
			"sectionPayload() returned an error: %v",
			err,
		)
	}

	actual, err := decodeHIIImage(
		payload,
		match.location,
	)
	if err != nil {
		t.Fatalf(
			"decode inserted boot logo: %v",
			err,
		)
	}

	actualBounds := actual.Bounds()
	replacementBounds := replacement.Bounds()

	if actualBounds.Dx() != replacementBounds.Dx() ||
		actualBounds.Dy() != replacementBounds.Dy() {
		t.Fatalf(
			"inserted logo dimensions = %dx%d, want %dx%d",
			actualBounds.Dx(),
			actualBounds.Dy(),
			replacementBounds.Dx(),
			replacementBounds.Dy(),
		)
	}

	assertFinalPERelocationMoved(
		t,
		originalPE,
		payload,
	)
}

func TestReplaceBootLogoRegeneratesFirmwareVolumePadding(t *testing.T) {
	const (
		replacementWidth  = 180
		replacementHeight = 116
		alignedFileBase   = 128 << 10
	)

	originalErasePolarity := uefi.Attributes.ErasePolarity
	uefi.Attributes.ErasePolarity = 0xff
	t.Cleanup(func() {
		uefi.Attributes.ErasePolarity = originalErasePolarity
	})

	sourceData := readFixture(t, "test.bmp")
	source, err := decodeBitmap(sourceData)
	if err != nil {
		t.Fatalf(
			"decode tests/test.bmp: %v",
			err,
		)
	}

	replacement := resizeFixtureImage(
		t,
		source,
		replacementWidth,
		replacementHeight,
	)

	block, palette, err := encodeHIIImage(replacement)
	if err != nil {
		t.Fatalf(
			"encode replacement HII image: %v",
			err,
		)
	}

	if len(block) == 0 ||
		block[0] != hiiImage24Bit ||
		palette != nil {
		t.Fatal(
			"replacement is not encoded as a 24-bit HII image",
		)
	}

	directory := t.TempDir()
	inputPath := filepath.Join(
		directory,
		"input.fd",
	)
	outputPath := filepath.Join(
		directory,
		"output.fd",
	)
	imagePath := filepath.Join(
		directory,
		"replacement.bmp",
	)
	replacementFFSPath := filepath.Join(
		directory,
		"replacement.ffs",
	)

	writeFixtureBitmap(
		t,
		imagePath,
		replacement,
	)

	createPaddingRegressionFirmware(
		t,
		inputPath,
	)

	copyFixture(
		t,
		"test.ffs",
		replacementFFSPath,
		0o644,
	)

	if err := replaceBootLogoImage(
		imagePath,
		replacementFFSPath,
		replacementFFSPath,
	); err != nil {
		t.Fatalf(
			"create expected replacement FFS: %v",
			err,
		)
	}

	expectedFFS := readFileForTest(
		t,
		replacementFFSPath,
	)

	initial := readFirmwareVolumeForTest(
		t,
		inputPath,
	)

	if len(initial.Files) != 3 {
		t.Fatalf(
			"initial firmware volume file count = %d, want 3",
			len(initial.Files),
		)
	}

	if initial.Files[0].Header.GUID != logoFileGUID {
		t.Fatalf(
			"initial first file GUID = %s, want %s",
			initial.Files[0].Header.GUID.String(),
			logoFileGUID.String(),
		)
	}

	initialPad := initial.Files[1]
	if initialPad.Header.Type != uefi.FVFileTypePad {
		t.Fatalf(
			"initial middle file type = %s, want %s",
			initialPad.Header.Type.String(),
			uefi.FVFileTypePad.String(),
		)
	}

	alignedFile := initial.Files[2]
	if alignment := alignedFile.Header.Attributes.GetAlignment(); alignment != alignedFileBase {
		t.Fatalf(
			"trailing file alignment = %d, want %d",
			alignment,
			alignedFileBase,
		)
	}

	replacementFile, ok := parseStandaloneFFS(expectedFFS)
	if !ok {
		t.Fatal("expected replacement is not a standalone FFS file")
	}

	correctFileOffset := alignedFirmwareFileOffset(
		initial.DataOffset+replacementFile.Header.ExtendedSize,
		alignedFile,
	)
	correctEnd := correctFileOffset +
		alignedFile.Header.ExtendedSize

	if correctEnd > initial.Length {
		t.Fatalf(
			"regenerated layout requires %#x bytes, firmware volume has %#x",
			correctEnd,
			initial.Length,
		)
	}

	stalePadOffset := uefi.Align8(
		initial.DataOffset +
			replacementFile.Header.ExtendedSize,
	)
	staleFileOffset := alignedFirmwareFileOffset(
		stalePadOffset+
			initialPad.Header.ExtendedSize,
		alignedFile,
	)
	staleEnd := staleFileOffset +
		alignedFile.Header.ExtendedSize

	if staleEnd <= initial.Length {
		t.Fatalf(
			"test precondition failed: retained PAD layout uses %#x bytes and still fits in %#x",
			staleEnd,
			initial.Length,
		)
	}

	if err := replaceBootLogo(
		imagePath,
		inputPath,
		outputPath,
	); err != nil {
		t.Fatalf(
			"replaceBootLogo() returned an error: %v",
			err,
		)
	}

	rebuilt := readFirmwareVolumeForTest(
		t,
		outputPath,
	)

	if len(rebuilt.Files) != 3 {
		t.Fatalf(
			"rebuilt firmware volume file count = %d, want 3",
			len(rebuilt.Files),
		)
	}

	if rebuilt.Files[0].Header.ExtendedSize != uint64(len(expectedFFS)) {
		t.Fatalf(
			"rebuilt LogoDxe size = %#x, want %#x",
			rebuilt.Files[0].Header.ExtendedSize,
			len(expectedFFS),
		)
	}

	rebuiltPad := rebuilt.Files[1]
	if rebuiltPad.Header.Type != uefi.FVFileTypePad {
		t.Fatalf(
			"rebuilt middle file type = %s, want %s",
			rebuiltPad.Header.Type.String(),
			uefi.FVFileTypePad.String(),
		)
	}

	if rebuiltPad.Header.ExtendedSize >=
		initialPad.Header.ExtendedSize {
		t.Fatalf(
			"rebuilt PAD size = %#x, want less than original %#x",
			rebuiltPad.Header.ExtendedSize,
			initialPad.Header.ExtendedSize,
		)
	}

	assertFixtureReplacement(
		t,
		outputPath,
		replacement,
	)
}

func TestReplaceBootLogoStandardROMResizes32MiBImage(t *testing.T) {
	const (
		replacementWidth  = 4073
		replacementHeight = 2746
		minimumBitmapSize = 32 << 20
		maximumBitmapSize = 33 << 20
	)

	sourceData := readFixture(t, "test.bmp")
	source, err := decodeBitmap(sourceData)
	if err != nil {
		t.Fatalf(
			"decode tests/test.bmp: %v",
			err,
		)
	}

	replacement := resizeFixtureImage(
		t,
		source,
		replacementWidth,
		replacementHeight,
	)

	directory := t.TempDir()
	imagePath := filepath.Join(
		directory,
		"replacement.bmp",
	)
	outputPath := filepath.Join(
		directory,
		"standard.rom",
	)

	writeFixtureBitmap(
		t,
		imagePath,
		replacement,
	)

	info, err := os.Stat(imagePath)
	if err != nil {
		t.Fatalf(
			"stat generated replacement bitmap: %v",
			err,
		)
	}

	if info.Size() < minimumBitmapSize ||
		info.Size() >= maximumBitmapSize {
		t.Fatalf(
			"generated replacement bitmap size = %d bytes, want at least %d and less than %d",
			info.Size(),
			minimumBitmapSize,
			maximumBitmapSize,
		)
	}

	var resizeInfo *logoResizeInfo

	err = replaceBootLogoWithReporter(
		imagePath,
		fixturePath("standard.rom"),
		outputPath,
		func(info logoResizeInfo) error {
			reported := info
			resizeInfo = &reported

			return nil
		},
	)
	if err != nil {
		t.Fatalf(
			"replaceBootLogo() returned an error: %v",
			err,
		)
	}

	firmware, err := readFirmware(outputPath)
	if err != nil {
		t.Fatalf(
			"updated standard ROM cannot be parsed: %v",
			err,
		)
	}

	match, err := findBootLogo(firmware)
	if err != nil {
		t.Fatalf(
			"updated standard ROM does not contain a logo: %v",
			err,
		)
	}

	if match.location.blockType != hiiImage24Bit {
		t.Fatalf(
			"resized HII image block type = %#x, want 24-bit",
			match.location.blockType,
		)
	}

	payload, err := sectionPayload(match.section)
	if err != nil {
		t.Fatalf(
			"sectionPayload() returned an error: %v",
			err,
		)
	}

	actual, err := decodeHIIImage(
		payload,
		match.location,
	)
	if err != nil {
		t.Fatalf(
			"decode resized boot logo: %v",
			err,
		)
	}

	actualBounds := actual.Bounds()
	replacementBounds := replacement.Bounds()

	if resizeInfo == nil {
		t.Fatal(
			"replaceBootLogo() did not report resizing the oversized logo",
		)
	}

	if resizeInfo.originalWidth != replacementBounds.Dx() ||
		resizeInfo.originalHeight != replacementBounds.Dy() {
		t.Fatalf(
			"reported original logo dimensions = %dx%d, want %dx%d",
			resizeInfo.originalWidth,
			resizeInfo.originalHeight,
			replacementBounds.Dx(),
			replacementBounds.Dy(),
		)
	}

	if resizeInfo.resizedWidth != actualBounds.Dx() ||
		resizeInfo.resizedHeight != actualBounds.Dy() {
		t.Fatalf(
			"reported resized logo dimensions = %dx%d, want %dx%d",
			resizeInfo.resizedWidth,
			resizeInfo.resizedHeight,
			actualBounds.Dx(),
			actualBounds.Dy(),
		)
	}

	if actualBounds.Dx() <= 0 ||
		actualBounds.Dy() <= 0 {
		t.Fatalf(
			"resized logo dimensions = %dx%d, want positive dimensions",
			actualBounds.Dx(),
			actualBounds.Dy(),
		)
	}

	if actualBounds.Dx() >= replacementBounds.Dx() ||
		actualBounds.Dy() >= replacementBounds.Dy() {
		t.Fatalf(
			"resized logo dimensions = %dx%d, want smaller than %dx%d",
			actualBounds.Dx(),
			actualBounds.Dy(),
			replacementBounds.Dx(),
			replacementBounds.Dy(),
		)
	}

	assertFixtureAspectRatio(
		t,
		actualBounds,
		replacementBounds,
	)
}

func TestReplaceBootLogoFixturesFromStandaloneFFS(t *testing.T) {
	outputModes := []struct {
		name     string
		separate bool
	}{
		{name: "in place"},
		{name: "separate output", separate: true},
	}

	for _, outputMode := range outputModes {
		outputMode := outputMode

		for _, fixture := range firmwareFixtures {
			fixture := fixture

			t.Run(outputMode.name+"/"+fixture.name, func(t *testing.T) {
				directory := t.TempDir()
				replacementPath, replacementData, replacementImage :=
					createFixtureReplacementFFS(t, directory)
				inputPath := filepath.Join(
					directory,
					"input-"+fixture.file,
				)
				original := copyFixture(
					t,
					fixture.file,
					inputPath,
					0o640,
				)
				originalLogoData := readLogoFileData(
					t,
					inputPath,
				)

				if bytes.Equal(
					originalLogoData,
					replacementData,
				) {
					t.Fatal(
						"test precondition failed: destination LogoDxe already matches the replacement FFS",
					)
				}

				outputPath := inputPath

				if outputMode.separate {
					outputPath = filepath.Join(
						directory,
						"output-"+fixture.file,
					)
				}

				if err := replaceBootLogo(
					replacementPath,
					inputPath,
					outputPath,
				); err != nil {
					t.Fatalf(
						"replaceBootLogo() returned an error: %v",
						err,
					)
				}

				if outputMode.separate {
					inputAfter, err := os.ReadFile(inputPath)
					if err != nil {
						t.Fatalf(
							"read unchanged input: %v",
							err,
						)
					}

					if !bytes.Equal(inputAfter, original) {
						t.Fatal(
							"FFS replacement modified the input while using a separate output",
						)
					}
				}

				updated, err := os.ReadFile(outputPath)
				if err != nil {
					t.Fatalf(
						"read FFS replacement output: %v",
						err,
					)
				}

				if bytes.Equal(updated, original) {
					t.Fatal(
						"FFS replacement did not modify the firmware",
					)
				}

				info, err := os.Stat(outputPath)
				if err != nil {
					t.Fatalf(
						"stat FFS replacement output: %v",
						err,
					)
				}

				if info.Mode().Perm() != 0o640 {
					t.Fatalf(
						"firmware mode = %#o, want %#o",
						info.Mode().Perm(),
						os.FileMode(0o640),
					)
				}

				assertLogoFileData(
					t,
					outputPath,
					replacementData,
				)
				assertFixtureReplacement(
					t,
					outputPath,
					replacementImage,
				)

				if fixture.file == "test.ffs" &&
					!bytes.Equal(updated, replacementData) {
					t.Fatal(
						"standalone FFS output does not exactly match the replacement FFS",
					)
				}
			})
		}
	}
}

func TestReplaceBootLogoFileRejectsDifferentGUID(t *testing.T) {
	replacementData := readFixture(t, "test.ffs")
	replacementFile, ok := parseStandaloneFFS(replacementData)
	if !ok {
		t.Fatal("parseStandaloneFFS() rejected tests/test.ffs")
	}

	replacementFile.Header.GUID[0] ^= 0xff

	err := replaceBootLogoFile(
		replacementFile,
		replacementData,
		fixturePath("test.rom"),
		filepath.Join(t.TempDir(), "output.rom"),
	)
	if err == nil {
		t.Fatal("replaceBootLogoFile() accepted a non-LogoDxe FFS file")
	}

	if !strings.Contains(err.Error(), "replacement FFS GUID") {
		t.Fatalf(
			"replaceBootLogoFile() error = %q, want GUID error",
			err,
		)
	}
}

func TestReplaceBootLogoFixturesFromSupportedFormats(t *testing.T) {
	referenceData := readFixture(t, "test.bmp")
	reference, err := decodeBitmap(referenceData)
	if err != nil {
		t.Fatalf("decode tests/test.bmp: %v", err)
	}
	mirrored := mirrorFixtureImageHorizontally(reference)
	for _, format := range fixtureImageFormats {
		format := format
		t.Run(format.name, func(t *testing.T) {
			inputData := encodeFixtureImage(t, format, mirrored)
			expectedBMP := decodeFixtureImageToBitmap(t, format, inputData)
			if bytes.Equal(expectedBMP, referenceData) {
				t.Fatal("transformed replacement unexpectedly matches the original logo")
			}
			for _, fixture := range firmwareFixtures {
				fixture := fixture
				t.Run(fixture.name, func(t *testing.T) {
					testReplacementFormat(t, fixture.file, format.extension, inputData, expectedBMP)
				})
			}
		})
	}
}

func TestParseStandaloneFFSFixture(t *testing.T) {
	data := readFixture(t, "test.ffs")
	file, ok := parseStandaloneFFS(data)
	if !ok {
		t.Fatal("parseStandaloneFFS() rejected tests/test.ffs")
	}
	if file.Header.GUID != logoFileGUID {
		t.Fatalf("parsed GUID = %s, want %s", file.Header.GUID.String(), logoFileGUID.String())
	}
	if _, err := findBootLogo(file); err != nil {
		t.Fatalf("findBootLogo() returned an error: %v", err)
	}
}

func TestParseStandaloneFFSRejectsROM(t *testing.T) {
	data := readFixture(t, "test.rom")
	if _, ok := parseStandaloneFFS(data); ok {
		t.Fatal("parseStandaloneFFS() accepted tests/test.rom")
	}
}

func TestParseStandaloneFFSRejectsTrailingData(t *testing.T) {
	data := append([]byte{}, readFixture(t, "test.ffs")...)
	data = append(data, 0xff)
	if _, ok := parseStandaloneFFS(data); ok {
		t.Fatal("parseStandaloneFFS() accepted trailing data")
	}
}

func TestParseStandaloneFFSRejectsShortData(t *testing.T) {
	data := make([]byte, uefi.FileHeaderMinLength-1)
	if _, ok := parseStandaloneFFS(data); ok {
		t.Fatal("parseStandaloneFFS() accepted incomplete data")
	}
}

func TestReadFirmwareParsesStandaloneFFSFixture(t *testing.T) {
	firmware, err := readFirmware(fixturePath("test.ffs"))
	if err != nil {
		t.Fatalf("readFirmware() returned an error: %v", err)
	}
	file, ok := firmware.(*uefi.File)
	if !ok {
		t.Fatalf("readFirmware() returned %T, want *uefi.File", firmware)
	}
	if file.Header.GUID != logoFileGUID {
		t.Fatalf("parsed GUID = %s, want %s", file.Header.GUID.String(), logoFileGUID.String())
	}
}

func TestReadFirmwareParsesROMFixture(t *testing.T) {
	firmware, err := readFirmware(fixturePath("test.rom"))
	if err != nil {
		t.Fatalf("readFirmware() returned an error: %v", err)
	}
	if firmware == nil {
		t.Fatal("readFirmware() returned nil")
	}
	if _, ok := firmware.(*uefi.File); ok {
		t.Fatal("readFirmware() treated the complete ROM as a standalone FFS file")
	}
	if _, err := findBootLogo(firmware); err != nil {
		t.Fatalf("findBootLogo() returned an error: %v", err)
	}
}

func TestFindBootLogoRejectsMissingLogoDxe(t *testing.T) {
	file := &uefi.File{}
	file.Header.GUID = logoFileGUID
	file.Header.GUID[0] ^= 0xff
	_, err := findBootLogo(file)
	if err == nil {
		t.Fatal("findBootLogo() accepted firmware without LogoDxe")
	}
	if !strings.Contains(err.Error(), "LogoDxe") {
		t.Fatalf("findBootLogo() error = %q, want LogoDxe error", err)
	}
}

func TestFindBootLogoRejectsLogoDxeWithoutHIIImage(t *testing.T) {
	file := &uefi.File{}
	file.Header.GUID = logoFileGUID
	_, err := findBootLogo(file)
	if err == nil {
		t.Fatal("findBootLogo() accepted LogoDxe without an HII image")
	}
	if !strings.Contains(err.Error(), "no HII boot logo") {
		t.Fatalf("findBootLogo() error = %q, want missing HII logo error", err)
	}
}

func TestSectionPayloadStandardHeader(t *testing.T) {
	payload := []byte{'P', 'E', 0x00, 0x00}
	section := &uefi.Section{}
	section.Header.Type = uefi.SectionTypePE32
	section.Header.Size = [3]uint8{byte(uefi.SectionMinLength + len(payload)), 0x00, 0x00}
	buffer := []byte{section.Header.Size[0], section.Header.Size[1], section.Header.Size[2], byte(uefi.SectionTypePE32)}
	buffer = append(buffer, payload...)
	section.SetBuf(buffer)
	actual, err := sectionPayload(section)
	if err != nil {
		t.Fatalf("sectionPayload() returned an error: %v", err)
	}
	if !bytes.Equal(actual, payload) {
		t.Fatalf("sectionPayload() = %v, want %v", actual, payload)
	}
}

func TestSectionPayloadExtendedHeader(t *testing.T) {
	payload := []byte{'P', 'E', 0x00, 0x00}
	section := &uefi.Section{}
	section.Header.Type = uefi.SectionTypePE32
	section.Header.Size = [3]uint8{0xff, 0xff, 0xff}
	buffer := []byte{0xff, 0xff, 0xff, byte(uefi.SectionTypePE32), 0x0c, 0x00, 0x00, 0x00}
	buffer = append(buffer, payload...)
	section.SetBuf(buffer)
	actual, err := sectionPayload(section)
	if err != nil {
		t.Fatalf("sectionPayload() returned an error: %v", err)
	}
	if !bytes.Equal(actual, payload) {
		t.Fatalf("sectionPayload() = %v, want %v", actual, payload)
	}
}

func TestSectionPayloadRejectsNilSection(t *testing.T) {
	if _, err := sectionPayload(nil); err == nil {
		t.Fatal("sectionPayload() accepted a nil section")
	}
}

func TestSectionPayloadRejectsShortHeader(t *testing.T) {
	section := &uefi.Section{}
	section.SetBuf([]byte{0x01, 0x02, 0x03})
	if _, err := sectionPayload(section); err == nil {
		t.Fatal("sectionPayload() accepted an incomplete header")
	}
}

func TestReadFirmwareRejectsEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.fd")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("WriteFile() returned an error: %v", err)
	}
	_, err := readFirmware(path)
	if err == nil {
		t.Fatal("readFirmware() accepted an empty file")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Fatalf("readFirmware() error = %q, want empty file error", err)
	}
}

func TestReadFirmwareRejectsMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.fd")
	_, err := readFirmware(path)
	if err == nil {
		t.Fatal("readFirmware() accepted a missing file")
	}
	if !strings.Contains(err.Error(), "read firmware") {
		t.Fatalf("readFirmware() error = %q, want read error", err)
	}
}

func TestFileMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "firmware.fd")
	if err := os.WriteFile(path, []byte("firmware"), 0o640); err != nil {
		t.Fatalf("WriteFile() returned an error: %v", err)
	}
	mode, err := fileMode(path)
	if err != nil {
		t.Fatalf("fileMode() returned an error: %v", err)
	}
	if mode != 0o640 {
		t.Fatalf("fileMode() = %#o, want %#o", mode, os.FileMode(0o640))
	}
}

func TestWriteFileAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "output.fd")
	expected := []byte("assembled firmware")
	if err := writeFileAtomic(path, expected, 0o640); err != nil {
		t.Fatalf("writeFileAtomic() returned an error: %v", err)
	}
	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() returned an error: %v", err)
	}
	if !bytes.Equal(actual, expected) {
		t.Fatalf("written data = %q, want %q", actual, expected)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() returned an error: %v", err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("written mode = %#o, want %#o", info.Mode().Perm(), os.FileMode(0o640))
	}
}

func TestWriteFileAtomicReplacesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "output.fd")
	if err := os.WriteFile(path, []byte("old firmware"), 0o600); err != nil {
		t.Fatalf("WriteFile() returned an error: %v", err)
	}
	expected := []byte("new firmware")
	if err := writeFileAtomic(path, expected, 0o644); err != nil {
		t.Fatalf("writeFileAtomic() returned an error: %v", err)
	}
	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() returned an error: %v", err)
	}
	if !bytes.Equal(actual, expected) {
		t.Fatalf("written data = %q, want %q", actual, expected)
	}
}

func TestWriteFileAtomicRejectsMissingDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "output.fd")
	err := writeFileAtomic(path, []byte("firmware"), 0o644)
	if err == nil {
		t.Fatal("writeFileAtomic() accepted a missing output directory")
	}
}

func fixturePath(name string) string {
	return filepath.Join("..", "tests", name)
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := fixturePath(name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %q: %v", path, err)
	}
	if len(data) == 0 {
		t.Fatalf("fixture %q is empty", path)
	}
	return data
}

func copyFixture(t *testing.T, name, destination string, mode os.FileMode) []byte {
	t.Helper()
	data := readFixture(t, name)
	if err := os.WriteFile(destination, data, mode); err != nil {
		t.Fatalf("write fixture copy %q: %v", destination, err)
	}
	return data
}

func createLogoFileWithExtraPE32(
	t *testing.T,
	path string,
) []byte {
	t.Helper()

	logoData := readFixture(
		t,
		"test.ffs",
	)
	logoFile, ok := parseStandaloneFFS(
		logoData,
	)
	if !ok {
		t.Fatal(
			"tests/test.ffs is not a standalone FFS file",
		)
	}

	match, err := findBootLogo(logoFile)
	if err != nil {
		t.Fatalf(
			"find boot logo in tests/test.ffs: %v",
			err,
		)
	}

	payload, err := sectionPayload(match.section)
	if err != nil {
		t.Fatalf(
			"read LogoDxe PE32 payload: %v",
			err,
		)
	}

	extraPE := append(
		[]byte(nil),
		payload...,
	)
	resource, err := findHIIResource(extraPE)
	if err != nil {
		t.Fatalf(
			"find HII resource in extra PE32: %v",
			err,
		)
	}

	resourceDirectoryOffset :=
		resource.directorySizeOffset - 4
	if resourceDirectoryOffset < 0 ||
		resource.directorySizeOffset+4 > len(extraPE) {
		t.Fatal(
			"PE resource directory entry is outside the image",
		)
	}

	for index := resourceDirectoryOffset; index < resource.directorySizeOffset+4; index++ {
		extraPE[index] = 0
	}

	images, err := findHIIImages(extraPE)
	if err != nil {
		t.Fatalf(
			"inspect extra PE32 after removing its resource directory: %v",
			err,
		)
	}
	if len(images) != 0 {
		t.Fatalf(
			"extra PE32 contains %d HII images, want none",
			len(images),
		)
	}

	extraSection := &uefi.Section{}
	extraSection.Header.Type = uefi.SectionTypePE32
	extraSection.SetBuf(extraPE)
	if err := extraSection.GenSecHeader(); err != nil {
		t.Fatalf(
			"assemble extra PE32 section: %v",
			err,
		)
	}

	logoFile.Sections = append(
		logoFile.Sections,
		extraSection,
	)
	logoFile.Modified = true

	if err := (&visitors.Assemble{}).Run(logoFile); err != nil {
		t.Fatalf(
			"assemble LogoDxe with extra PE32: %v",
			err,
		)
	}

	if err := os.WriteFile(
		path,
		logoFile.Buf(),
		0o644,
	); err != nil {
		t.Fatalf(
			"write LogoDxe with extra PE32: %v",
			err,
		)
	}

	parsed, err := readFirmware(path)
	if err != nil {
		t.Fatalf(
			"read LogoDxe with extra PE32: %v",
			err,
		)
	}
	if _, err := findBootLogo(parsed); err != nil {
		t.Fatalf(
			"find boot logo with extra PE32 present: %v",
			err,
		)
	}

	return extraPE
}

func createPaddingRegressionFirmware(
	t *testing.T,
	path string,
) {
	t.Helper()

	const (
		firmwareVolumeSize = 0x21000
		blockSize          = 0x1000
		alignedFileSize    = 0x40
	)

	logoData := readFixture(
		t,
		"test.ffs",
	)
	logoFile, ok := parseStandaloneFFS(
		logoData,
	)
	if !ok {
		t.Fatal("tests/test.ffs is not a standalone FFS file")
	}

	alignedFile := newAlignedRawFile(
		t,
		"11111111-2222-3333-4444-555555555555",
		alignedFileSize,
		128<<10,
	)

	fv := &uefi.FirmwareVolume{}
	fv.FileSystemGUID = *uefi.FFS2
	fv.Signature = binary.LittleEndian.Uint32(
		[]byte("_FVH"),
	)
	fv.Attributes = 0x0004feff
	fv.Revision = 2
	fv.Blocks = []uefi.Block{
		{
			Count: firmwareVolumeSize / blockSize,
			Size:  blockSize,
		},
		{},
	}
	fv.HeaderLen = uint16(
		uefi.FirmwareVolumeFixedHeaderSize +
			binary.Size(uefi.Block{})*len(fv.Blocks),
	)
	fv.DataOffset = uint64(fv.HeaderLen)
	fv.Length = firmwareVolumeSize
	fv.Files = []*uefi.File{
		logoFile,
		alignedFile,
	}

	header := new(bytes.Buffer)
	if err := binary.Write(
		header,
		binary.LittleEndian,
		fv.FirmwareVolumeFixedHeader,
	); err != nil {
		t.Fatalf(
			"write firmware volume header: %v",
			err,
		)
	}

	for _, block := range fv.Blocks {
		if err := binary.Write(
			header,
			binary.LittleEndian,
			block,
		); err != nil {
			t.Fatalf(
				"write firmware volume block map: %v",
				err,
			)
		}
	}

	buffer := header.Bytes()
	sum, err := uefi.Checksum16(
		buffer[:fv.HeaderLen],
	)
	if err != nil {
		t.Fatalf(
			"checksum firmware volume header: %v",
			err,
		)
	}

	binary.LittleEndian.PutUint16(
		buffer[50:52],
		0-sum,
	)

	empty := make(
		[]byte,
		fv.Length-uint64(len(buffer)),
	)
	uefi.Erase(
		empty,
		uefi.Attributes.ErasePolarity,
	)
	fv.SetBuf(
		append(buffer, empty...),
	)

	if err := (&visitors.Assemble{}).Run(fv); err != nil {
		t.Fatalf(
			"assemble padding regression firmware: %v",
			err,
		)
	}

	if err := os.WriteFile(
		path,
		fv.Buf(),
		0o644,
	); err != nil {
		t.Fatalf(
			"write padding regression firmware: %v",
			err,
		)
	}
}

func newAlignedRawFile(
	t *testing.T,
	fileGUID string,
	size uint64,
	alignment uint64,
) *uefi.File {
	t.Helper()

	if alignment != 128<<10 {
		t.Fatalf(
			"unsupported test file alignment: %d",
			alignment,
		)
	}

	if size < uefi.FileHeaderMinLength {
		t.Fatalf(
			"raw file size = %#x, want at least %#x",
			size,
			uefi.FileHeaderMinLength,
		)
	}

	file := &uefi.File{}
	file.Header.GUID = *guid.MustParse(fileGUID)
	file.Header.Type = uefi.FVFileTypeRaw
	file.Header.Attributes = 0x02
	file.Type = file.Header.Type.String()
	file.SetSize(
		size,
		true,
	)
	file.Header.SetState(
		uefi.FileStateValid,
	)

	data := make(
		[]byte,
		size-file.HeaderLen(),
	)
	for index := range data {
		data[index] = byte(index)
	}

	if err := file.ChecksumAndAssemble(data); err != nil {
		t.Fatalf(
			"assemble aligned raw file: %v",
			err,
		)
	}

	file.Modified = false

	return file
}

func alignedFirmwareFileOffset(
	fileOffset uint64,
	file *uefi.File,
) uint64 {
	alignedOffset := uefi.Align8(
		fileOffset,
	)
	alignBase := file.Header.Attributes.GetAlignment()

	if alignBase == 1 {
		return alignedOffset
	}

	headerLength := file.HeaderLen()
	fileDataOffset := uefi.Align(
		alignedOffset+headerLength,
		alignBase,
	)
	newOffset := fileDataOffset -
		headerLength

	if gap := newOffset - alignedOffset; gap >= 8 &&
		gap < uefi.FileHeaderMinLength {
		fileDataOffset = uefi.Align(
			fileDataOffset+1,
			alignBase,
		)
		newOffset = fileDataOffset -
			headerLength
	}

	return newOffset
}

func readFirmwareVolumeForTest(
	t *testing.T,
	path string,
) *uefi.FirmwareVolume {
	t.Helper()

	data := readFileForTest(
		t,
		path,
	)

	fv, err := uefi.NewFirmwareVolume(
		data,
		0,
		false,
	)
	if err != nil {
		t.Fatalf(
			"parse firmware volume %q: %v",
			path,
			err,
		)
	}

	return fv
}

func readFileForTest(
	t *testing.T,
	path string,
) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf(
			"read file %q: %v",
			path,
			err,
		)
	}

	if len(data) == 0 {
		t.Fatalf(
			"file %q is empty",
			path,
		)
	}

	return data
}

func resizeFixtureImage(
	t *testing.T,
	source image.Image,
	width int,
	height int,
) *image.NRGBA {
	t.Helper()

	if source == nil {
		t.Fatal("source image is nil")
	}

	sourceBounds := source.Bounds()
	sourceWidth := sourceBounds.Dx()
	sourceHeight := sourceBounds.Dy()

	if sourceWidth <= 0 || sourceHeight <= 0 {
		t.Fatalf(
			"source dimensions = %dx%d, want positive dimensions",
			sourceWidth,
			sourceHeight,
		)
	}

	if width <= 0 || height <= 0 {
		t.Fatalf(
			"destination dimensions = %dx%d, want positive dimensions",
			width,
			height,
		)
	}

	output := image.NewNRGBA(
		image.Rect(0, 0, width, height),
	)

	for y := 0; y < height; y++ {
		sourceY := sourceBounds.Min.Y +
			y*sourceHeight/height

		for x := 0; x < width; x++ {
			sourceX := sourceBounds.Min.X +
				x*sourceWidth/width

			output.Set(
				x,
				y,
				source.At(sourceX, sourceY),
			)
		}
	}

	return output
}

func assertFixtureAspectRatio(
	t *testing.T,
	actual image.Rectangle,
	expected image.Rectangle,
) {
	t.Helper()

	actualWidth := actual.Dx()
	actualHeight := actual.Dy()
	expectedWidth := expected.Dx()
	expectedHeight := expected.Dy()

	difference :=
		actualWidth*expectedHeight -
			actualHeight*expectedWidth

	if difference < 0 {
		difference = -difference
	}

	tolerance := expectedWidth
	if expectedHeight > tolerance {
		tolerance = expectedHeight
	}

	if difference > tolerance {
		t.Fatalf(
			"resized logo aspect ratio %dx%d differs from source %dx%d",
			actualWidth,
			actualHeight,
			expectedWidth,
			expectedHeight,
		)
	}
}

func fixtureReplacementImage(width, height int) *image.NRGBA {
	output := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			output.SetNRGBA(x, y, color.NRGBA{
				R: uint8(20 + x*17),
				G: uint8(30 + y*23),
				B: uint8(40 + (x+y)*11),
				A: 0xff,
			})
		}
	}
	return output
}

func writeFixtureBitmap(t *testing.T, path string, source image.Image) {
	t.Helper()
	data, err := encodeBitmap(source)
	if err != nil {
		t.Fatalf("encodeBitmap() returned an error: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile() returned an error: %v", err)
	}
}

func createFixtureReplacementFFS(
	t *testing.T,
	directory string,
) (string, []byte, image.Image) {
	t.Helper()

	replacementPath := filepath.Join(
		directory,
		"replacement-source.bin",
	)
	original := copyFixture(
		t,
		"test.ffs",
		replacementPath,
		0o644,
	)
	replacementImage := fixtureReplacementImage(11, 7)
	imagePath := filepath.Join(
		directory,
		"replacement-source.bmp",
	)
	writeFixtureBitmap(t, imagePath, replacementImage)

	if err := replaceBootLogoImage(
		imagePath,
		replacementPath,
		replacementPath,
	); err != nil {
		t.Fatalf(
			"create replacement FFS: %v",
			err,
		)
	}

	replacementData, err := os.ReadFile(replacementPath)
	if err != nil {
		t.Fatalf(
			"read replacement FFS: %v",
			err,
		)
	}

	if bytes.Equal(replacementData, original) {
		t.Fatal("generated replacement FFS is unchanged")
	}

	replacementFile, ok := parseStandaloneFFS(replacementData)
	if !ok {
		t.Fatal("generated replacement is not a standalone FFS file")
	}

	if replacementFile.Header.GUID != logoFileGUID {
		t.Fatalf(
			"generated replacement GUID = %s, want %s",
			replacementFile.Header.GUID.String(),
			logoFileGUID.String(),
		)
	}

	return replacementPath, replacementData, replacementImage
}

func readLogoFileData(
	t *testing.T,
	firmwarePath string,
) []byte {
	t.Helper()

	firmware, err := readFirmware(firmwarePath)
	if err != nil {
		t.Fatalf(
			"firmware cannot be parsed: %v",
			err,
		)
	}

	file, err := findLogoFile(firmware)
	if err != nil {
		t.Fatalf(
			"firmware does not contain LogoDxe: %v",
			err,
		)
	}

	return append([]byte(nil), file.Buf()...)
}

func assertLogoFileData(
	t *testing.T,
	firmwarePath string,
	expected []byte,
) {
	t.Helper()

	actual := readLogoFileData(t, firmwarePath)

	if !bytes.Equal(actual, expected) {
		t.Fatal(
			"destination LogoDxe FFS does not exactly match the replacement FFS",
		)
	}
}

func assertFixtureReplacement(t *testing.T, firmwarePath string, expected image.Image) {
	t.Helper()
	firmware, err := readFirmware(firmwarePath)
	if err != nil {
		t.Fatalf("updated firmware cannot be parsed: %v", err)
	}
	match, err := findBootLogo(firmware)
	if err != nil {
		t.Fatalf("updated firmware does not contain a logo: %v", err)
	}
	payload, err := sectionPayload(match.section)
	if err != nil {
		t.Fatalf("sectionPayload() returned an error: %v", err)
	}
	actual, err := decodeHIIImage(payload, match.location)
	if err != nil {
		t.Fatalf("decodeHIIImage() returned an error: %v", err)
	}
	assertFixtureImagesEqual(t, actual, expected)
}

func assertFixtureImagesEqual(t *testing.T, actual, expected image.Image) {
	t.Helper()
	if actual == nil {
		t.Fatal("actual image is nil")
	}
	if expected == nil {
		t.Fatal("expected image is nil")
	}
	actualBounds := actual.Bounds()
	expectedBounds := expected.Bounds()
	if actualBounds.Dx() != expectedBounds.Dx() || actualBounds.Dy() != expectedBounds.Dy() {
		t.Fatalf("image dimensions = %dx%d, want %dx%d", actualBounds.Dx(), actualBounds.Dy(), expectedBounds.Dx(), expectedBounds.Dy())
	}
	for y := 0; y < expectedBounds.Dy(); y++ {
		for x := 0; x < expectedBounds.Dx(); x++ {
			actualColor := color.NRGBAModel.Convert(actual.At(actualBounds.Min.X+x, actualBounds.Min.Y+y)).(color.NRGBA)
			expectedColor := color.NRGBAModel.Convert(expected.At(expectedBounds.Min.X+x, expectedBounds.Min.Y+y)).(color.NRGBA)
			if actualColor.R != expectedColor.R || actualColor.G != expectedColor.G || actualColor.B != expectedColor.B {
				t.Fatalf("pixel (%d,%d) = (%d,%d,%d), want (%d,%d,%d)", x, y, actualColor.R, actualColor.G, actualColor.B, expectedColor.R, expectedColor.G, expectedColor.B)
			}
		}
	}
}

func testReplacementFormat(t *testing.T, fixtureName, extension string, inputData, expectedBMP []byte) {
	t.Helper()
	directory := t.TempDir()
	firmwarePath := filepath.Join(directory, fixtureName)
	originalFirmware := copyFixture(t, fixtureName, firmwarePath, 0o644)
	imagePath := filepath.Join(directory, "replacement"+extension)
	if err := os.WriteFile(imagePath, inputData, 0o644); err != nil {
		t.Fatalf("write replacement image: %v", err)
	}
	if err := replaceBootLogo(imagePath, firmwarePath, firmwarePath); err != nil {
		t.Fatalf("replaceBootLogo() returned an error: %v", err)
	}
	modifiedFirmware, err := os.ReadFile(firmwarePath)
	if err != nil {
		t.Fatalf("read modified firmware: %v", err)
	}
	if bytes.Equal(modifiedFirmware, originalFirmware) {
		t.Fatal("replacement did not modify the firmware")
	}
	extractedPath := filepath.Join(directory, "extracted.bmp")
	if err := extractBootLogo(firmwarePath, extractedPath); err != nil {
		t.Fatalf("extract modified boot logo: %v", err)
	}
	extractedBMP, err := os.ReadFile(extractedPath)
	if err != nil {
		t.Fatalf("read extracted bitmap: %v", err)
	}
	if !bytes.Equal(extractedBMP, expectedBMP) {
		t.Fatal("extracted logo does not match the decoded replacement image")
	}
}

func encodeFixtureImage(t *testing.T, format fixtureImageFormat, source image.Image) []byte {
	t.Helper()
	var output bytes.Buffer
	if err := format.encode(&output, source); err != nil {
		t.Fatalf("encode %s replacement: %v", format.name, err)
	}
	if output.Len() == 0 {
		t.Fatalf("encoded %s replacement is empty", format.name)
	}
	return output.Bytes()
}

func decodeFixtureImageToBitmap(t *testing.T, format fixtureImageFormat, data []byte) []byte {
	t.Helper()
	decoded, err := format.decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("independently decode %s replacement: %v", format.name, err)
	}
	bitmap, err := encodeBitmap(decoded)
	if err != nil {
		t.Fatalf("encode expected %s bitmap: %v", format.name, err)
	}
	return bitmap
}

func mirrorFixtureImageHorizontally(source image.Image) *image.NRGBA {
	bounds := source.Bounds()
	output := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			sourceX := bounds.Max.X - 1 - x
			sourceY := bounds.Min.Y + y
			output.Set(x, y, source.At(sourceX, sourceY))
		}
	}
	return output
}

func readBootLogoPE(
	t *testing.T,
	firmwarePath string,
) []byte {
	t.Helper()

	firmware, err := readFirmware(firmwarePath)
	if err != nil {
		t.Fatalf(
			"read firmware %q: %v",
			firmwarePath,
			err,
		)
	}

	match, err := findBootLogo(firmware)
	if err != nil {
		t.Fatalf(
			"find boot logo in %q: %v",
			firmwarePath,
			err,
		)
	}

	payload, err := sectionPayload(match.section)
	if err != nil {
		t.Fatalf(
			"read LogoDxe PE32 payload from %q: %v",
			firmwarePath,
			err,
		)
	}

	return append([]byte(nil), payload...)
}

func assertFinalPERelocationMoved(
	t *testing.T,
	original []byte,
	updated []byte,
) {
	t.Helper()

	originalResource, err := findHIIResource(original)
	if err != nil {
		t.Fatalf(
			"find original HII resource: %v",
			err,
		)
	}

	updatedResource, err := findHIIResource(updated)
	if err != nil {
		t.Fatalf(
			"find updated HII resource: %v",
			err,
		)
	}

	originalRelocation := findPESectionByNameForTest(
		t,
		originalResource.sections,
		".reloc",
	)

	updatedRelocation := findPESectionByNameForTest(
		t,
		updatedResource.sections,
		".reloc",
	)

	if updatedResource.section.virtualAddress !=
		originalResource.section.virtualAddress {
		t.Fatalf(
			".rsrc RVA changed from %#x to %#x",
			originalResource.section.virtualAddress,
			updatedResource.section.virtualAddress,
		)
	}

	if updatedResource.section.virtualSize <=
		originalResource.section.virtualSize {
		t.Fatalf(
			".rsrc did not grow: old size %#x, new size %#x",
			originalResource.section.virtualSize,
			updatedResource.section.virtualSize,
		)
	}

	if updatedRelocation.virtualAddress <=
		originalRelocation.virtualAddress {
		t.Fatalf(
			".reloc did not move forward: old RVA %#x, new RVA %#x",
			originalRelocation.virtualAddress,
			updatedRelocation.virtualAddress,
		)
	}

	if updatedResource.sectionAlignment < 0x1000 &&
		updatedResource.sectionAlignment ==
			updatedResource.fileAlignment &&
		updatedRelocation.rawOffset !=
			updatedRelocation.virtualAddress {
		t.Fatalf(
			"low-alignment .reloc raw offset %#x does not match RVA %#x",
			updatedRelocation.rawOffset,
			updatedRelocation.virtualAddress,
		)
	}

	originalRelocationStart :=
		int(originalRelocation.rawOffset)

	originalRelocationEnd :=
		originalRelocationStart +
			int(originalRelocation.rawSize)

	updatedRelocationStart :=
		int(updatedRelocation.rawOffset)

	updatedRelocationEnd :=
		updatedRelocationStart +
			int(updatedRelocation.rawSize)

	originalRelocationData :=
		original[originalRelocationStart:originalRelocationEnd]

	updatedRelocationData :=
		updated[updatedRelocationStart:updatedRelocationEnd]

	if !bytes.Equal(
		updatedRelocationData,
		originalRelocationData,
	) {
		t.Fatal(
			"moving .reloc changed its contents",
		)
	}

	originalDirectoryOffset :=
		originalResource.relocationRVA -
			originalRelocation.virtualAddress

	updatedDirectoryOffset :=
		updatedResource.relocationRVA -
			updatedRelocation.virtualAddress

	if updatedDirectoryOffset != originalDirectoryOffset {
		t.Fatalf(
			"base relocation directory offset changed from %#x to %#x",
			originalDirectoryOffset,
			updatedDirectoryOffset,
		)
	}

	for _, originalSection := range originalResource.sections {
		if originalSection.name == ".rsrc" ||
			originalSection.name == ".reloc" {
			continue
		}

		updatedSection := findPESectionByNameForTest(
			t,
			updatedResource.sections,
			originalSection.name,
		)

		if updatedSection.virtualAddress !=
			originalSection.virtualAddress {
			t.Fatalf(
				"section %q RVA changed from %#x to %#x",
				originalSection.name,
				originalSection.virtualAddress,
				updatedSection.virtualAddress,
			)
		}
	}
}

func findPESectionByNameForTest(
	t *testing.T,
	sections []peSectionInfo,
	name string,
) peSectionInfo {
	t.Helper()

	for _, section := range sections {
		if section.name == name {
			return section
		}
	}

	t.Fatalf(
		"PE section %q was not found",
		name,
	)

	return peSectionInfo{}
}
