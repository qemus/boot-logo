package main

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/linuxboot/fiano/pkg/uefi"
)

func TestParseBitmapAt(t *testing.T) {
	image := testBitmap(2, 2)

	location, err := parseBitmapAt(image, 0)
	if err != nil {
		t.Fatalf("parseBitmapAt() returned an error: %v", err)
	}

	if location.offset != 0 {
		t.Fatalf(
			"parseBitmapAt() offset = %d, want 0",
			location.offset,
		)
	}

	if location.length != len(image) {
		t.Fatalf(
			"parseBitmapAt() length = %d, want %d",
			location.length,
			len(image),
		)
	}
}

func TestFindEmbeddedBitmaps(t *testing.T) {
	first := testBitmap(2, 2)
	second := testBitmap(3, 1)

	data := append([]byte{0x01, 0x02, 0x03}, first...)
	data = append(data, []byte{0x04, 0x05}...)
	secondOffset := len(data)
	data = append(data, second...)

	locations, err := findEmbeddedBitmaps(data)
	if err != nil {
		t.Fatalf(
			"findEmbeddedBitmaps() returned an error: %v",
			err,
		)
	}

	if len(locations) != 2 {
		t.Fatalf(
			"findEmbeddedBitmaps() found %d images, want 2",
			len(locations),
		)
	}

	if locations[0].offset != 3 {
		t.Errorf(
			"first bitmap offset = %d, want 3",
			locations[0].offset,
		)
	}

	if locations[0].length != len(first) {
		t.Errorf(
			"first bitmap length = %d, want %d",
			locations[0].length,
			len(first),
		)
	}

	if locations[1].offset != secondOffset {
		t.Errorf(
			"second bitmap offset = %d, want %d",
			locations[1].offset,
			secondOffset,
		)
	}

	if locations[1].length != len(second) {
		t.Errorf(
			"second bitmap length = %d, want %d",
			locations[1].length,
			len(second),
		)
	}
}

func TestFindEmbeddedBitmapsIgnoresInvalidSignature(t *testing.T) {
	data := []byte{
		0x00,
		'B',
		'M',
		0x01,
		0x02,
		0x03,
		0x04,
	}

	locations, err := findEmbeddedBitmaps(data)
	if err != nil {
		t.Fatalf(
			"findEmbeddedBitmaps() returned an error: %v",
			err,
		)
	}

	if len(locations) != 0 {
		t.Fatalf(
			"findEmbeddedBitmaps() found %d images, want 0",
			len(locations),
		)
	}
}

func TestReplaceEmbeddedBitmap(t *testing.T) {
	original := testBitmap(2, 2)
	replacement := testBitmap(4, 3)

	prefix := []byte{0x10, 0x20, 0x30}
	suffix := []byte{0x40, 0x50, 0x60}

	payload := append([]byte{}, prefix...)
	payload = append(payload, original...)
	payload = append(payload, suffix...)

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

	locations, err := findEmbeddedBitmaps(section.Buf())
	if err != nil {
		t.Fatalf(
			"findEmbeddedBitmaps() returned an error: %v",
			err,
		)
	}

	if len(locations) != 1 {
		t.Fatalf(
			"findEmbeddedBitmaps() found %d images, want 1",
			len(locations),
		)
	}

	if err := replaceEmbeddedBitmap(
		section,
		locations[0],
		replacement,
	); err != nil {
		t.Fatalf(
			"replaceEmbeddedBitmap() returned an error: %v",
			err,
		)
	}

	if !section.Modified {
		t.Fatal("replaceEmbeddedBitmap() did not mark the section modified")
	}

	updatedLocations, err := findEmbeddedBitmaps(section.Buf())
	if err != nil {
		t.Fatalf(
			"findEmbeddedBitmaps() after replacement returned an error: %v",
			err,
		)
	}

	if len(updatedLocations) != 1 {
		t.Fatalf(
			"updated section contains %d images, want 1",
			len(updatedLocations),
		)
	}

	location := updatedLocations[0]
	actual := section.Buf()[
		location.offset : location.offset+location.length
	]

	if !bytes.Equal(actual, replacement) {
		t.Fatal("replacement bitmap does not match the input bitmap")
	}

	headerLength := 4

	if section.Header.Size == [3]uint8{0xFF, 0xFF, 0xFF} {
		headerLength = 8
	}

	expectedPayload := append([]byte{}, prefix...)
	expectedPayload = append(expectedPayload, replacement...)
	expectedPayload = append(expectedPayload, suffix...)

	if !bytes.Equal(section.Buf()[headerLength:], expectedPayload) {
		t.Fatal("data surrounding the bitmap was not preserved")
	}
}

func TestReplaceEmbeddedBitmapRejectsNonRawSection(t *testing.T) {
	image := testBitmap(2, 2)

	section, err := uefi.CreateSection(
		uefi.SectionTypePE32,
		image,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("CreateSection() returned an error: %v", err)
	}

	if err := section.GenSecHeader(); err != nil {
		t.Fatalf("GenSecHeader() returned an error: %v", err)
	}

	locations, err := findEmbeddedBitmaps(section.Buf())
	if err != nil {
		t.Fatalf(
			"findEmbeddedBitmaps() returned an error: %v",
			err,
		)
	}

	if len(locations) != 1 {
		t.Fatalf(
			"findEmbeddedBitmaps() found %d images, want 1",
			len(locations),
		)
	}

	err = replaceEmbeddedBitmap(
		section,
		locations[0],
		image,
	)

	if err == nil {
		t.Fatal(
			"replaceEmbeddedBitmap() accepted a non-raw section",
		)
	}
}

func testBitmap(width uint32, height uint32) []byte {
	const (
		fileHeaderSize = uint32(14)
		infoHeaderSize = uint32(40)
		pixelOffset    = fileHeaderSize + infoHeaderSize
		bitsPerPixel   = uint32(24)
	)

	rowBits := uint64(width) * bitsPerPixel
	rowSize := uint32(((rowBits + 31) / 32) * 4)
	pixelSize := rowSize * height
	fileSize := pixelOffset + pixelSize

	image := make([]byte, fileSize)

	copy(image[0:2], bitmapSignature)

	binary.LittleEndian.PutUint32(
		image[2:6],
		fileSize,
	)

	binary.LittleEndian.PutUint32(
		image[10:14],
		pixelOffset,
	)

	binary.LittleEndian.PutUint32(
		image[14:18],
		infoHeaderSize,
	)

	binary.LittleEndian.PutUint32(
		image[18:22],
		width,
	)

	binary.LittleEndian.PutUint32(
		image[22:26],
		height,
	)

	binary.LittleEndian.PutUint16(
		image[26:28],
		1,
	)

	binary.LittleEndian.PutUint16(
		image[28:30],
		uint16(bitsPerPixel),
	)

	binary.LittleEndian.PutUint32(
		image[34:38],
		pixelSize,
	)

	for index := pixelOffset; index < fileSize; index++ {
		image[index] = byte(index)
	}

	return image
}
