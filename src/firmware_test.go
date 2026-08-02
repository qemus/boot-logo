package main

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/linuxboot/fiano/pkg/uefi"
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
