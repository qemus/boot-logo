package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/linuxboot/fiano/pkg/uefi"
)

func TestFindBootLogo(t *testing.T) {
	image := testBitmap(4, 3)
	section := testRawSection(t, image)
	file := testLogoFile(section)

	match, err := findBootLogo(file)
	if err != nil {
		t.Fatalf("findBootLogo() returned an error: %v", err)
	}

	if match.section != section {
		t.Fatal("findBootLogo() returned the wrong section")
	}

	start := match.location.offset
	end := start + match.location.length

	if start < 0 || end > len(section.Buf()) {
		t.Fatalf(
			"bitmap location [%d:%d] is outside section of size %d",
			start,
			end,
			len(section.Buf()),
		)
	}

	if !bytes.Equal(section.Buf()[start:end], image) {
		t.Fatal("located bitmap does not match the embedded image")
	}
}

func TestFindBootLogoInNestedSection(t *testing.T) {
	image := testBitmap(3, 2)
	rawSection := testRawSection(t, image)

	containerSection := &uefi.Section{
		Encapsulated: []*uefi.TypedFirmware{
			uefi.MakeTyped(rawSection),
		},
	}

	file := testLogoFile(containerSection)

	match, err := findBootLogo(file)
	if err != nil {
		t.Fatalf("findBootLogo() returned an error: %v", err)
	}

	if match.section != rawSection {
		t.Fatal("findBootLogo() did not return the nested raw section")
	}
}

func TestFindBootLogoRejectsMissingLogoDxe(t *testing.T) {
	image := testBitmap(2, 2)
	section := testRawSection(t, image)

	file := testLogoFile(section)
	file.Header.GUID[0] ^= 0xff

	_, err := findBootLogo(file)
	if err == nil {
		t.Fatal("findBootLogo() accepted firmware without LogoDxe")
	}

	if !strings.Contains(err.Error(), "LogoDxe") {
		t.Fatalf(
			"findBootLogo() error = %q, want LogoDxe error",
			err,
		)
	}
}

func TestFindBootLogoRejectsMissingBitmap(t *testing.T) {
	section := testRawSection(
		t,
		[]byte("this section does not contain a bitmap"),
	)

	file := testLogoFile(section)

	_, err := findBootLogo(file)
	if err == nil {
		t.Fatal("findBootLogo() accepted LogoDxe without a bitmap")
	}

	if !strings.Contains(err.Error(), "no bitmap") {
		t.Fatalf(
			"findBootLogo() error = %q, want missing bitmap error",
			err,
		)
	}
}

func TestFindBootLogoRejectsMultipleBitmaps(t *testing.T) {
	first := testRawSection(t, testBitmap(2, 2))
	second := testRawSection(t, testBitmap(3, 3))

	file := testLogoFile(first, second)

	_, err := findBootLogo(file)
	if err == nil {
		t.Fatal("findBootLogo() accepted multiple bitmaps")
	}

	if !strings.Contains(err.Error(), "multiple bitmaps") {
		t.Fatalf(
			"findBootLogo() error = %q, want multiple bitmap error",
			err,
		)
	}
}

func TestFindBootLogoIgnoresInvalidBitmapSignature(t *testing.T) {
	invalid := []byte{
		0x00,
		'B',
		'M',
		0x01,
		0x02,
		0x03,
	}

	valid := testBitmap(2, 2)

	payload := append([]byte{}, invalid...)
	payload = append(payload, valid...)

	section := testRawSection(t, payload)
	file := testLogoFile(section)

	match, err := findBootLogo(file)
	if err != nil {
		t.Fatalf("findBootLogo() returned an error: %v", err)
	}

	start := match.location.offset
	end := start + match.location.length
	actual := section.Buf()[start:end]

	if !bytes.Equal(actual, valid) {
		t.Fatal("findBootLogo() selected the invalid bitmap signature")
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
		t.Fatalf(
			"readFirmware() error = %q, want empty file error",
			err,
		)
	}
}

func TestReadFirmwareRejectsMissingFile(t *testing.T) {
	path := filepath.Join(
		t.TempDir(),
		"missing.fd",
	)

	_, err := readFirmware(path)
	if err == nil {
		t.Fatal("readFirmware() accepted a missing file")
	}

	if !strings.Contains(err.Error(), "read firmware") {
		t.Fatalf(
			"readFirmware() error = %q, want read error",
			err,
		)
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
		t.Fatalf(
			"fileMode() = %#o, want %#o",
			mode,
			os.FileMode(0o640),
		)
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
		t.Fatalf(
			"written data = %q, want %q",
			actual,
			expected,
		)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() returned an error: %v", err)
	}

	if info.Mode().Perm() != 0o640 {
		t.Fatalf(
			"written mode = %#o, want %#o",
			info.Mode().Perm(),
			os.FileMode(0o640),
		)
	}
}

func TestWriteFileAtomicReplacesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "output.fd")

	if err := os.WriteFile(
		path,
		[]byte("old firmware"),
		0o600,
	); err != nil {
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
		t.Fatalf(
			"written data = %q, want %q",
			actual,
			expected,
		)
	}
}

func TestWriteFileAtomicRejectsMissingDirectory(t *testing.T) {
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

func testLogoFile(
	sections ...*uefi.Section,
) *uefi.File {
	file := &uefi.File{
		Sections: sections,
	}

	file.Header.GUID = logoFileGUID

	return file
}

func testRawSection(
	t *testing.T,
	payload []byte,
) *uefi.Section {
	t.Helper()

	section, err := uefi.CreateSection(
		uefi.SectionTypeRaw,
		payload,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("CreateSection() returned an error: %v", err)
	}

	if err := section.GenSecHeader(); err != nil {
		t.Fatalf("GenSecHeader() returned an error: %v", err)
	}

	return section
}
