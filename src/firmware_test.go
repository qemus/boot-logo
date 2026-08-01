package main

import (
	"bytes"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/linuxboot/fiano/pkg/uefi"
)

var firmwareFixtures = []struct {
	name string
	file string
}{
	{
		name: "standalone FFS",
		file: "test.ffs",
	},
	{
		name: "complete ROM",
		file: "test.rom",
	},
}

func TestReadFirmwareFixtures(t *testing.T) {
	for _, fixture := range firmwareFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			path := fixturePath(fixture.file)

			if _, err := os.Stat(path); err != nil {
				t.Fatalf(
					"fixture %q is unavailable: %v",
					path,
					err,
				)
			}

			firmware, err := readFirmware(path)
			if err != nil {
				t.Fatalf(
					"readFirmware() returned an error: %v",
					err,
				)
			}

			if firmware == nil {
				t.Fatal(
					"readFirmware() returned nil",
				)
			}
		})
	}
}

func TestFindBootLogoFixtures(t *testing.T) {
	for _, fixture := range firmwareFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			firmware, err := readFirmware(
				fixturePath(fixture.file),
			)
			if err != nil {
				t.Fatalf(
					"readFirmware() returned an error: %v",
					err,
				)
			}

			match, err := findBootLogo(firmware)
			if err != nil {
				t.Fatalf(
					"findBootLogo() returned an error: %v",
					err,
				)
			}

			if match.section == nil {
				t.Fatal(
					"findBootLogo() returned a nil section",
				)
			}

			if match.section.Header.Type != uefi.SectionTypePE32 {
				t.Fatalf(
					"section type = %s, want %s",
					match.section.Header.Type.String(),
					uefi.SectionTypePE32.String(),
				)
			}

			payload, err := sectionPayload(
				match.section,
			)
			if err != nil {
				t.Fatalf(
					"sectionPayload() returned an error: %v",
					err,
				)
			}

			logo, err := decodeHIIImage(
				payload,
				match.location,
			)
			if err != nil {
				t.Fatalf(
					"decodeHIIImage() returned an error: %v",
					err,
				)
			}

			bounds := logo.Bounds()

			if bounds.Dx() <= 0 ||
				bounds.Dy() <= 0 {
				t.Fatalf(
					"logo dimensions = %dx%d, want positive dimensions",
					bounds.Dx(),
					bounds.Dy(),
				)
			}
		})
	}
}

func TestExtractBootLogoFixtures(t *testing.T) {
	for _, fixture := range firmwareFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			output := filepath.Join(
				t.TempDir(),
				"logo.bmp",
			)

			err := extractBootLogo(
				fixturePath(fixture.file),
				output,
			)
			if err != nil {
				t.Fatalf(
					"extractBootLogo() returned an error: %v",
					err,
				)
			}

			data, err := os.ReadFile(output)
			if err != nil {
				t.Fatalf(
					"ReadFile() returned an error: %v",
					err,
				)
			}

			if len(data) < len(bitmapSignature) ||
				!bytes.Equal(
					data[:len(bitmapSignature)],
					bitmapSignature,
				) {
				t.Fatal(
					"extracted image does not have a BMP signature",
				)
			}

			logo, err := decodeBitmap(data)
			if err != nil {
				t.Fatalf(
					"decodeBitmap() returned an error: %v",
					err,
				)
			}

			bounds := logo.Bounds()

			if bounds.Dx() <= 0 ||
				bounds.Dy() <= 0 {
				t.Fatalf(
					"extracted dimensions = %dx%d, want positive dimensions",
					bounds.Dx(),
					bounds.Dy(),
				)
			}
		})
	}
}

func TestExtractBootLogoFixturesMatchesHIIParser(
	t *testing.T,
) {
	for _, fixture := range firmwareFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			firmware, err := readFirmware(
				fixturePath(fixture.file),
			)
			if err != nil {
				t.Fatalf(
					"readFirmware() returned an error: %v",
					err,
				)
			}

			match, err := findBootLogo(firmware)
			if err != nil {
				t.Fatalf(
					"findBootLogo() returned an error: %v",
					err,
				)
			}

			payload, err := sectionPayload(
				match.section,
			)
			if err != nil {
				t.Fatalf(
					"sectionPayload() returned an error: %v",
					err,
				)
			}

			expected, err := decodeHIIImage(
				payload,
				match.location,
			)
			if err != nil {
				t.Fatalf(
					"decodeHIIImage() returned an error: %v",
					err,
				)
			}

			output := filepath.Join(
				t.TempDir(),
				"logo.bmp",
			)

			if err := extractBootLogo(
				fixturePath(fixture.file),
				output,
			); err != nil {
				t.Fatalf(
					"extractBootLogo() returned an error: %v",
					err,
				)
			}

			data, err := os.ReadFile(output)
			if err != nil {
				t.Fatalf(
					"ReadFile() returned an error: %v",
					err,
				)
			}

			actual, err := decodeBitmap(data)
			if err != nil {
				t.Fatalf(
					"decodeBitmap() returned an error: %v",
					err,
				)
			}

			assertFixtureImagesEqual(
				t,
				actual,
				expected,
			)
		})
	}
}

func TestReplaceBootLogoFixturesInPlace(
	t *testing.T,
) {
	for _, fixture := range firmwareFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			directory := t.TempDir()

			firmwarePath := filepath.Join(
				directory,
				fixture.file,
			)

			original := copyFixture(
				t,
				fixture.file,
				firmwarePath,
				0o640,
			)

			replacement := fixtureReplacementImage(
				7,
				5,
			)

			imagePath := filepath.Join(
				directory,
				"replacement.bmp",
			)

			writeFixtureBitmap(
				t,
				imagePath,
				replacement,
			)

			if err := replaceBootLogo(
				imagePath,
				firmwarePath,
				firmwarePath,
			); err != nil {
				t.Fatalf(
					"replaceBootLogo() returned an error: %v",
					err,
				)
			}

			updated, err := os.ReadFile(
				firmwarePath,
			)
			if err != nil {
				t.Fatalf(
					"ReadFile() returned an error: %v",
					err,
				)
			}

			if bytes.Equal(updated, original) {
				t.Fatal(
					"replacement did not modify the firmware",
				)
			}

			info, err := os.Stat(firmwarePath)
			if err != nil {
				t.Fatalf(
					"Stat() returned an error: %v",
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

			assertFixtureReplacement(
				t,
				firmwarePath,
				replacement,
			)
		})
	}
}

func TestReplaceBootLogoFixturesSeparateOutput(
	t *testing.T,
) {
	for _, fixture := range firmwareFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			directory := t.TempDir()

			inputPath := filepath.Join(
				directory,
				"input-"+fixture.file,
			)

			original := copyFixture(
				t,
				fixture.file,
				inputPath,
				0o644,
			)

			outputPath := filepath.Join(
				directory,
				"output-"+fixture.file,
			)

			replacement := fixtureReplacementImage(
				9,
				6,
			)

			imagePath := filepath.Join(
				directory,
				"replacement.bmp",
			)

			writeFixtureBitmap(
				t,
				imagePath,
				replacement,
			)

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

			inputAfter, err := os.ReadFile(
				inputPath,
			)
			if err != nil {
				t.Fatalf(
					"ReadFile() returned an error: %v",
					err,
				)
			}

			if !bytes.Equal(
				inputAfter,
				original,
			) {
				t.Fatal(
					"replacement modified the input while using a separate output",
				)
			}

			output, err := os.ReadFile(
				outputPath,
			)
			if err != nil {
				t.Fatalf(
					"ReadFile() returned an error: %v",
					err,
				)
			}

			if bytes.Equal(output, original) {
				t.Fatal(
					"separate output is unchanged",
				)
			}

			assertFixtureReplacement(
				t,
				outputPath,
				replacement,
			)
		})
	}
}

func TestParseStandaloneFFSFixture(
	t *testing.T,
) {
	data := readFixture(
		t,
		"test.ffs",
	)

	file, ok := parseStandaloneFFS(data)
	if !ok {
		t.Fatal(
			"parseStandaloneFFS() rejected tests/test.ffs",
		)
	}

	if file.Header.GUID != logoFileGUID {
		t.Fatalf(
			"parsed GUID = %s, want %s",
			file.Header.GUID.String(),
			logoFileGUID.String(),
		)
	}

	if _, err := findBootLogo(file); err != nil {
		t.Fatalf(
			"findBootLogo() returned an error: %v",
			err,
		)
	}
}

func TestParseStandaloneFFSRejectsROM(
	t *testing.T,
) {
	data := readFixture(
		t,
		"test.rom",
	)

	if _, ok := parseStandaloneFFS(data); ok {
		t.Fatal(
			"parseStandaloneFFS() accepted tests/test.rom",
		)
	}
}

func TestParseStandaloneFFSRejectsTrailingData(
	t *testing.T,
) {
	data := append(
		[]byte{},
		readFixture(t, "test.ffs")...,
	)

	data = append(data, 0xff)

	if _, ok := parseStandaloneFFS(data); ok {
		t.Fatal(
			"parseStandaloneFFS() accepted trailing data",
		)
	}
}

func TestParseStandaloneFFSRejectsShortData(
	t *testing.T,
) {
	data := make(
		[]byte,
		uefi.FileHeaderMinLength-1,
	)

	if _, ok := parseStandaloneFFS(data); ok {
		t.Fatal(
			"parseStandaloneFFS() accepted incomplete data",
		)
	}
}

func TestReadFirmwareParsesStandaloneFFSFixture(
	t *testing.T,
) {
	firmware, err := readFirmware(
		fixturePath("test.ffs"),
	)
	if err != nil {
		t.Fatalf(
			"readFirmware() returned an error: %v",
			err,
		)
	}

	file, ok := firmware.(*uefi.File)
	if !ok {
		t.Fatalf(
			"readFirmware() returned %T, want *uefi.File",
			firmware,
		)
	}

	if file.Header.GUID != logoFileGUID {
		t.Fatalf(
			"parsed GUID = %s, want %s",
			file.Header.GUID.String(),
			logoFileGUID.String(),
		)
	}
}

func TestReadFirmwareParsesROMFixture(
	t *testing.T,
) {
	firmware, err := readFirmware(
		fixturePath("test.rom"),
	)
	if err != nil {
		t.Fatalf(
			"readFirmware() returned an error: %v",
			err,
		)
	}

	if firmware == nil {
		t.Fatal(
			"readFirmware() returned nil",
		)
	}

	if _, ok := firmware.(*uefi.File); ok {
		t.Fatal(
			"readFirmware() treated the complete ROM as a standalone FFS file",
		)
	}

	if _, err := findBootLogo(firmware); err != nil {
		t.Fatalf(
			"findBootLogo() returned an error: %v",
			err,
		)
	}
}

func TestFindBootLogoRejectsMissingLogoDxe(
	t *testing.T,
) {
	file := &uefi.File{}
	file.Header.GUID = logoFileGUID
	file.Header.GUID[0] ^= 0xff

	_, err := findBootLogo(file)
	if err == nil {
		t.Fatal(
			"findBootLogo() accepted firmware without LogoDxe",
		)
	}

	if !strings.Contains(
		err.Error(),
		"LogoDxe",
	) {
		t.Fatalf(
			"findBootLogo() error = %q, want LogoDxe error",
			err,
		)
	}
}

func TestFindBootLogoRejectsLogoDxeWithoutHIIImage(
	t *testing.T,
) {
	file := &uefi.File{}
	file.Header.GUID = logoFileGUID

	_, err := findBootLogo(file)
	if err == nil {
		t.Fatal(
			"findBootLogo() accepted LogoDxe without an HII image",
		)
	}

	if !strings.Contains(
		err.Error(),
		"no HII boot logo",
	) {
		t.Fatalf(
			"findBootLogo() error = %q, want missing HII logo error",
			err,
		)
	}
}

func TestSectionPayloadStandardHeader(
	t *testing.T,
) {
	payload := []byte{
		'P',
		'E',
		0x00,
		0x00,
	}

	section := &uefi.Section{}
	section.Header.Type = uefi.SectionTypePE32
	section.Header.Size = [3]uint8{
		byte(
			uefi.SectionMinLength +
				len(payload),
		),
		0x00,
		0x00,
	}

	buffer := []byte{
		section.Header.Size[0],
		section.Header.Size[1],
		section.Header.Size[2],
		byte(uefi.SectionTypePE32),
	}

	buffer = append(
		buffer,
		payload...,
	)

	section.SetBuf(buffer)

	actual, err := sectionPayload(section)
	if err != nil {
		t.Fatalf(
			"sectionPayload() returned an error: %v",
			err,
		)
	}

	if !bytes.Equal(actual, payload) {
		t.Fatalf(
			"sectionPayload() = %v, want %v",
			actual,
			payload,
		)
	}
}

func TestSectionPayloadExtendedHeader(
	t *testing.T,
) {
	payload := []byte{
		'P',
		'E',
		0x00,
		0x00,
	}

	section := &uefi.Section{}
	section.Header.Type = uefi.SectionTypePE32
	section.Header.Size = [3]uint8{
		0xff,
		0xff,
		0xff,
	}

	buffer := []byte{
		0xff,
		0xff,
		0xff,
		byte(uefi.SectionTypePE32),
		0x0c,
		0x00,
		0x00,
		0x00,
	}

	buffer = append(
		buffer,
		payload...,
	)

	section.SetBuf(buffer)

	actual, err := sectionPayload(section)
	if err != nil {
		t.Fatalf(
			"sectionPayload() returned an error: %v",
			err,
		)
	}

	if !bytes.Equal(actual, payload) {
		t.Fatalf(
			"sectionPayload() = %v, want %v",
			actual,
			payload,
		)
	}
}

func TestSectionPayloadRejectsNilSection(
	t *testing.T,
) {
	if _, err := sectionPayload(nil); err == nil {
		t.Fatal(
			"sectionPayload() accepted a nil section",
		)
	}
}

func TestSectionPayloadRejectsShortHeader(
	t *testing.T,
) {
	section := &uefi.Section{}
	section.SetBuf([]byte{
		0x01,
		0x02,
		0x03,
	})

	if _, err := sectionPayload(section); err == nil {
		t.Fatal(
			"sectionPayload() accepted an incomplete header",
		)
	}
}

func TestReadFirmwareRejectsEmptyFile(
	t *testing.T,
) {
	path := filepath.Join(
		t.TempDir(),
		"empty.fd",
	)

	if err := os.WriteFile(
		path,
		nil,
		0o644,
	); err != nil {
		t.Fatalf(
			"WriteFile() returned an error: %v",
			err,
		)
	}

	_, err := readFirmware(path)
	if err == nil {
		t.Fatal(
			"readFirmware() accepted an empty file",
		)
	}

	if !strings.Contains(
		err.Error(),
		"empty",
	) {
		t.Fatalf(
			"readFirmware() error = %q, want empty file error",
			err,
		)
	}
}

func TestReadFirmwareRejectsMissingFile(
	t *testing.T,
) {
	path := filepath.Join(
		t.TempDir(),
		"missing.fd",
	)

	_, err := readFirmware(path)
	if err == nil {
		t.Fatal(
			"readFirmware() accepted a missing file",
		)
	}

	if !strings.Contains(
		err.Error(),
		"read firmware",
	) {
		t.Fatalf(
			"readFirmware() error = %q, want read error",
			err,
		)
	}
}

func TestFileMode(t *testing.T) {
	path := filepath.Join(
		t.TempDir(),
		"firmware.fd",
	)

	if err := os.WriteFile(
		path,
		[]byte("firmware"),
		0o640,
	); err != nil {
		t.Fatalf(
			"WriteFile() returned an error: %v",
			err,
		)
	}

	mode, err := fileMode(path)
	if err != nil {
		t.Fatalf(
			"fileMode() returned an error: %v",
			err,
		)
	}

	if mode != 0o640 {
		t.Fatalf(
			"fileMode() = %#o, want %#o",
			mode,
			os.FileMode(0o640),
		)
	}
}

func TestWriteFileAtomic(t *testing.T) {
	path := filepath.Join(
		t.TempDir(),
		"output.fd",
	)

	expected := []byte(
		"assembled firmware",
	)

	if err := writeFileAtomic(
		path,
		expected,
		0o640,
	); err != nil {
		t.Fatalf(
			"writeFileAtomic() returned an error: %v",
			err,
		)
	}

	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf(
			"ReadFile() returned an error: %v",
			err,
		)
	}

	if !bytes.Equal(actual, expected) {
		t.Fatalf(
			"written data = %q, want %q",
			actual,
			expected,
		)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf(
			"Stat() returned an error: %v",
			err,
		)
	}

	if info.Mode().Perm() != 0o640 {
		t.Fatalf(
			"written mode = %#o, want %#o",
			info.Mode().Perm(),
			os.FileMode(0o640),
		)
	}
}

func TestWriteFileAtomicReplacesExistingFile(
	t *testing.T,
) {
	path := filepath.Join(
		t.TempDir(),
		"output.fd",
	)

	if err := os.WriteFile(
		path,
		[]byte("old firmware"),
		0o600,
	); err != nil {
		t.Fatalf(
			"WriteFile() returned an error: %v",
			err,
		)
	}

	expected := []byte(
		"new firmware",
	)

	if err := writeFileAtomic(
		path,
		expected,
		0o644,
	); err != nil {
		t.Fatalf(
			"writeFileAtomic() returned an error: %v",
			err,
		)
	}

	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf(
			"ReadFile() returned an error: %v",
			err,
		)
	}

	if !bytes.Equal(actual, expected) {
		t.Fatalf(
			"written data = %q, want %q",
			actual,
			expected,
		)
	}
}

func TestWriteFileAtomicRejectsMissingDirectory(
	t *testing.T,
) {
	path := filepath.Join(
		t.TempDir(),
		"missing",
		"output.fd",
	)

	err := writeFileAtomic(
		path,
		[]byte("firmware"),
		0o644,
	)

	if err == nil {
		t.Fatal(
			"writeFileAtomic() accepted a missing output directory",
		)
	}
}

func fixturePath(name string) string {
	return filepath.Join(
		"..",
		"tests",
		name,
	)
}

func readFixture(
	t *testing.T,
	name string,
) []byte {
	t.Helper()

	path := fixturePath(name)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf(
			"read fixture %q: %v",
			path,
			err,
		)
	}

	if len(data) == 0 {
		t.Fatalf(
			"fixture %q is empty",
			path,
		)
	}

	return data
}

func copyFixture(
	t *testing.T,
	name string,
	destination string,
	mode os.FileMode,
) []byte {
	t.Helper()

	data := readFixture(t, name)

	if err := os.WriteFile(
		destination,
		data,
		mode,
	); err != nil {
		t.Fatalf(
			"write fixture copy %q: %v",
			destination,
			err,
		)
	}

	return data
}

func fixtureReplacementImage(
	width int,
	height int,
) *image.NRGBA {
	output := image.NewNRGBA(
		image.Rect(
			0,
			0,
			width,
			height,
		),
	)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			output.SetNRGBA(
				x,
				y,
				color.NRGBA{
					R: uint8(
						20 + x*17,
					),
					G: uint8(
						30 + y*23,
					),
					B: uint8(
						40 +
							(x+y)*11,
					),
					A: 0xff,
				},
			)
		}
	}

	return output
}

func writeFixtureBitmap(
	t *testing.T,
	path string,
	source image.Image,
) {
	t.Helper()

	data, err := encodeBitmap(source)
	if err != nil {
		t.Fatalf(
			"encodeBitmap() returned an error: %v",
			err,
		)
	}

	if err := os.WriteFile(
		path,
		data,
		0o644,
	); err != nil {
		t.Fatalf(
			"WriteFile() returned an error: %v",
			err,
		)
	}
}

func assertFixtureReplacement(
	t *testing.T,
	firmwarePath string,
	expected image.Image,
) {
	t.Helper()

	firmware, err := readFirmware(
		firmwarePath,
	)
	if err != nil {
		t.Fatalf(
			"updated firmware cannot be parsed: %v",
			err,
		)
	}

	match, err := findBootLogo(firmware)
	if err != nil {
		t.Fatalf(
			"updated firmware does not contain a logo: %v",
			err,
		)
	}

	payload, err := sectionPayload(
		match.section,
	)
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
			"decodeHIIImage() returned an error: %v",
			err,
		)
	}

	assertFixtureImagesEqual(
		t,
		actual,
		expected,
	)
}

func assertFixtureImagesEqual(
	t *testing.T,
	actual image.Image,
	expected image.Image,
) {
	t.Helper()

	if actual == nil {
		t.Fatal(
			"actual image is nil",
		)
	}

	if expected == nil {
		t.Fatal(
			"expected image is nil",
		)
	}

	actualBounds := actual.Bounds()
	expectedBounds := expected.Bounds()

	if actualBounds.Dx() != expectedBounds.Dx() ||
		actualBounds.Dy() != expectedBounds.Dy() {
		t.Fatalf(
			"image dimensions = %dx%d, want %dx%d",
			actualBounds.Dx(),
			actualBounds.Dy(),
			expectedBounds.Dx(),
			expectedBounds.Dy(),
		)
	}

	for y := 0; y < expectedBounds.Dy(); y++ {
		for x := 0; x < expectedBounds.Dx(); x++ {
			actualColor := color.NRGBAModel.Convert(
				actual.At(
					actualBounds.Min.X+x,
					actualBounds.Min.Y+y,
				),
			).(color.NRGBA)

			expectedColor := color.NRGBAModel.Convert(
				expected.At(
					expectedBounds.Min.X+x,
					expectedBounds.Min.Y+y,
				),
			).(color.NRGBA)

			if actualColor.R != expectedColor.R ||
				actualColor.G != expectedColor.G ||
				actualColor.B != expectedColor.B {
				t.Fatalf(
					"pixel (%d,%d) = (%d,%d,%d), want (%d,%d,%d)",
					x,
					y,
					actualColor.R,
					actualColor.G,
					actualColor.B,
					expectedColor.R,
					expectedColor.G,
					expectedColor.B,
				)
			}
		}
	}
}
