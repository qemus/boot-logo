package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/linuxboot/fiano/pkg/uefi"
)

func TestReadBitmapNormalizesBMP(t *testing.T) {
	source := testBitmap32TopDown(t, 3, 2)
	path := filepath.Join(t.TempDir(), "image.bmp")

	if err := os.WriteFile(path, source, 0o644); err != nil {
		t.Fatalf("WriteFile() returned an error: %v", err)
	}

	result, err := readBitmap(path)
	if err != nil {
		t.Fatalf("readBitmap() returned an error: %v", err)
	}

	assertCanonicalBitmap(t, result, 3, 2)

	if bytes.Equal(result, source) {
		t.Fatal("readBitmap() did not normalize the source bitmap")
	}

	decoded, err := decodeBitmap(result)
	if err != nil {
		t.Fatalf("decodeBitmap() returned an error: %v", err)
	}

	assertPixel(
		t,
		decoded,
		0,
		0,
		color.NRGBA{
			R: 0xff,
			G: 0x00,
			B: 0x00,
			A: 0xff,
		},
	)

	assertPixel(
		t,
		decoded,
		2,
		1,
		color.NRGBA{
			R: 0x00,
			G: 0xff,
			B: 0xff,
			A: 0xff,
		},
	)
}

func TestReadBitmapConvertsPNG(t *testing.T) {
	source := testSourceImage()
	path := filepath.Join(t.TempDir(), "image.dat")

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create() returned an error: %v", err)
	}

	if err := png.Encode(file, source); err != nil {
		_ = file.Close()
		t.Fatalf("png.Encode() returned an error: %v", err)
	}

	if err := file.Close(); err != nil {
		t.Fatalf("Close() returned an error: %v", err)
	}

	result, err := readBitmap(path)
	if err != nil {
		t.Fatalf("readBitmap() returned an error: %v", err)
	}

	assertCanonicalBitmap(t, result, 3, 2)

	decoded, err := decodeBitmap(result)
	if err != nil {
		t.Fatalf("decodeBitmap() returned an error: %v", err)
	}

	assertPixel(
		t,
		decoded,
		0,
		0,
		color.NRGBA{
			R: 0xff,
			G: 0x00,
			B: 0x00,
			A: 0xff,
		},
	)

	assertPixel(
		t,
		decoded,
		2,
		1,
		color.NRGBA{
			R: 0x00,
			G: 0xff,
			B: 0xff,
			A: 0xff,
		},
	)
}

func TestReadBitmapConvertsJPEG(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 8, 8))

	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			source.SetNRGBA(
				x,
				y,
				color.NRGBA{
					R: 0x24,
					G: 0x68,
					B: 0xa0,
					A: 0xff,
				},
			)
		}
	}

	path := filepath.Join(t.TempDir(), "image.bin")

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create() returned an error: %v", err)
	}

	if err := jpeg.Encode(
		file,
		source,
		&jpeg.Options{
			Quality: 100,
		},
	); err != nil {
		_ = file.Close()
		t.Fatalf("jpeg.Encode() returned an error: %v", err)
	}

	if err := file.Close(); err != nil {
		t.Fatalf("Close() returned an error: %v", err)
	}

	result, err := readBitmap(path)
	if err != nil {
		t.Fatalf("readBitmap() returned an error: %v", err)
	}

	assertCanonicalBitmap(t, result, 8, 8)

	decoded, err := decodeBitmap(result)
	if err != nil {
		t.Fatalf("decodeBitmap() returned an error: %v", err)
	}

	pixel := color.NRGBAModel.Convert(
		decoded.At(4, 4),
	).(color.NRGBA)

	assertColorNear(
		t,
		pixel,
		color.NRGBA{
			R: 0x24,
			G: 0x68,
			B: 0xa0,
			A: 0xff,
		},
		4,
	)
}

func TestReadBitmapDetectsFormatByContents(t *testing.T) {
	source := testSourceImage()
	path := filepath.Join(t.TempDir(), "image.bmp")

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create() returned an error: %v", err)
	}

	if err := png.Encode(file, source); err != nil {
		_ = file.Close()
		t.Fatalf("png.Encode() returned an error: %v", err)
	}

	if err := file.Close(); err != nil {
		t.Fatalf("Close() returned an error: %v", err)
	}

	result, err := readBitmap(path)
	if err != nil {
		t.Fatalf("readBitmap() returned an error: %v", err)
	}

	assertCanonicalBitmap(t, result, 3, 2)
}

func TestReadBitmapRejectsUnknownFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image.dat")

	if err := os.WriteFile(
		path,
		[]byte("not an image"),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile() returned an error: %v", err)
	}

	_, err := readBitmap(path)
	if err == nil {
		t.Fatal("readBitmap() accepted an unsupported format")
	}
}

func TestReadBitmapRejectsEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image.bmp")

	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("WriteFile() returned an error: %v", err)
	}

	_, err := readBitmap(path)
	if err == nil {
		t.Fatal("readBitmap() accepted an empty file")
	}
}

func TestEncodeBitmap(t *testing.T) {
	source := testSourceImage()

	result, err := encodeBitmap(source)
	if err != nil {
		t.Fatalf("encodeBitmap() returned an error: %v", err)
	}

	assertCanonicalBitmap(t, result, 3, 2)

	decoded, err := decodeBitmap(result)
	if err != nil {
		t.Fatalf("decodeBitmap() returned an error: %v", err)
	}

	for y := 0; y < source.Bounds().Dy(); y++ {
		for x := 0; x < source.Bounds().Dx(); x++ {
			expected := color.NRGBAModel.Convert(
				source.At(x, y),
			).(color.NRGBA)

			assertPixel(t, decoded, x, y, expected)
		}
	}
}

func TestEncodeBitmapUsesImageBounds(t *testing.T) {
	source := image.NewNRGBA(
		image.Rect(10, 20, 12, 22),
	)

	source.SetNRGBA(
		10,
		20,
		color.NRGBA{
			R: 0xff,
			A: 0xff,
		},
	)

	source.SetNRGBA(
		11,
		21,
		color.NRGBA{
			B: 0xff,
			A: 0xff,
		},
	)

	result, err := encodeBitmap(source)
	if err != nil {
		t.Fatalf("encodeBitmap() returned an error: %v", err)
	}

	assertCanonicalBitmap(t, result, 2, 2)

	decoded, err := decodeBitmap(result)
	if err != nil {
		t.Fatalf("decodeBitmap() returned an error: %v", err)
	}

	assertPixel(
		t,
		decoded,
		0,
		0,
		color.NRGBA{
			R: 0xff,
			A: 0xff,
		},
	)

	assertPixel(
		t,
		decoded,
		1,
		1,
		color.NRGBA{
			B: 0xff,
			A: 0xff,
		},
	)
}

func TestParseBitmapAt(t *testing.T) {
	bitmap := testBitmap(2, 2)

	location, err := parseBitmapAt(bitmap, 0)
	if err != nil {
		t.Fatalf("parseBitmapAt() returned an error: %v", err)
	}

	if location.offset != 0 {
		t.Fatalf(
			"parseBitmapAt() offset = %d, want 0",
			location.offset,
		)
	}

	if location.length != len(bitmap) {
		t.Fatalf(
			"parseBitmapAt() length = %d, want %d",
			location.length,
			len(bitmap),
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
		t.Fatal(
			"replaceEmbeddedBitmap() did not mark the section modified",
		)
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
	start := location.offset
	end := start + location.length
	actual := section.Buf()[start:end]

	if !bytes.Equal(actual, replacement) {
		t.Fatal("replacement bitmap does not match the input bitmap")
	}

	headerLength := 4

	if section.Header.Size == [3]uint8{0xff, 0xff, 0xff} {
		headerLength = 8
	}

	expectedPayload := append([]byte{}, prefix...)
	expectedPayload = append(expectedPayload, replacement...)
	expectedPayload = append(expectedPayload, suffix...)

	if !bytes.Equal(
		section.Buf()[headerLength:],
		expectedPayload,
	) {
		t.Fatal("data surrounding the bitmap was not preserved")
	}
}

func TestReplaceEmbeddedBitmapRejectsNonRawSection(t *testing.T) {
	bitmap := testBitmap(2, 2)

	section, err := uefi.CreateSection(
		uefi.SectionTypePE32,
		bitmap,
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
		bitmap,
	)

	if err == nil {
		t.Fatal(
			"replaceEmbeddedBitmap() accepted a non-raw section",
		)
	}
}

func assertCanonicalBitmap(
	t *testing.T,
	bitmap []byte,
	width int,
	height int,
) {
	t.Helper()

	metadata, err := inspectBitmapAt(bitmap, 0)
	if err != nil {
		t.Fatalf("inspectBitmapAt() returned an error: %v", err)
	}

	if metadata.location.length != len(bitmap) {
		t.Errorf(
			"bitmap length = %d, header reports %d",
			len(bitmap),
			metadata.location.length,
		)
	}

	if metadata.width != width {
		t.Errorf(
			"bitmap width = %d, want %d",
			metadata.width,
			width,
		)
	}

	if metadata.height != height {
		t.Errorf(
			"bitmap height = %d, want %d",
			metadata.height,
			height,
		)
	}

	if metadata.topDown {
		t.Error("bitmap is top-down, want bottom-up")
	}

	if metadata.bitsPerPixel != 24 {
		t.Errorf(
			"bitmap color depth = %d, want 24",
			metadata.bitsPerPixel,
		)
	}

	if metadata.pixelOffset != bitmapDataOffset {
		t.Errorf(
			"bitmap pixel offset = %d, want %d",
			metadata.pixelOffset,
			bitmapDataOffset,
		)
	}

	dibSize := binary.LittleEndian.Uint32(bitmap[14:18])

	if dibSize != bitmapInfoHeaderSize {
		t.Errorf(
			"bitmap DIB header size = %d, want %d",
			dibSize,
			bitmapInfoHeaderSize,
		)
	}

	compression := binary.LittleEndian.Uint32(bitmap[30:34])

	if compression != 0 {
		t.Errorf(
			"bitmap compression = %d, want 0",
			compression,
		)
	}
}

func assertPixel(
	t *testing.T,
	source image.Image,
	x int,
	y int,
	expected color.NRGBA,
) {
	t.Helper()

	actual := color.NRGBAModel.Convert(
		source.At(x, y),
	).(color.NRGBA)

	if actual != expected {
		t.Errorf(
			"pixel (%d,%d) = %#v, want %#v",
			x,
			y,
			actual,
			expected,
		)
	}
}

func assertColorNear(
	t *testing.T,
	actual color.NRGBA,
	expected color.NRGBA,
	tolerance uint8,
) {
	t.Helper()

	if channelDifference(actual.R, expected.R) > tolerance ||
		channelDifference(actual.G, expected.G) > tolerance ||
		channelDifference(actual.B, expected.B) > tolerance ||
		channelDifference(actual.A, expected.A) > tolerance {
		t.Errorf(
			"color = %#v, want approximately %#v",
			actual,
			expected,
		)
	}
}

func channelDifference(first uint8, second uint8) uint8 {
	if first > second {
		return first - second
	}

	return second - first
}

func testSourceImage() *image.NRGBA {
	source := image.NewNRGBA(
		image.Rect(0, 0, 3, 2),
	)

	colors := [][]color.NRGBA{
		{
			{R: 0xff, A: 0xff},
			{G: 0xff, A: 0xff},
			{B: 0xff, A: 0xff},
		},
		{
			{R: 0xff, G: 0xff, A: 0xff},
			{R: 0xff, B: 0xff, A: 0xff},
			{G: 0xff, B: 0xff, A: 0xff},
		},
	}

	for y, row := range colors {
		for x, value := range row {
			source.SetNRGBA(x, y, value)
		}
	}

	return source
}

func testBitmap(
	width uint32,
	height uint32,
) []byte {
	const (
		fileHeaderSize = uint32(14)
		infoHeaderSize = uint32(40)
		pixelOffset    = fileHeaderSize + infoHeaderSize
		bitsPerPixel   = uint32(24)
	)

	rowBits := uint64(width) * uint64(bitsPerPixel)
	rowSize := uint32(((rowBits + 31) / 32) * 4)
	pixelSize := rowSize * height
	fileSize := pixelOffset + pixelSize

	bitmap := make([]byte, fileSize)

	copy(bitmap[0:2], bitmapSignature)

	binary.LittleEndian.PutUint32(
		bitmap[2:6],
		fileSize,
	)

	binary.LittleEndian.PutUint32(
		bitmap[10:14],
		pixelOffset,
	)

	binary.LittleEndian.PutUint32(
		bitmap[14:18],
		infoHeaderSize,
	)

	binary.LittleEndian.PutUint32(
		bitmap[18:22],
		width,
	)

	binary.LittleEndian.PutUint32(
		bitmap[22:26],
		height,
	)

	binary.LittleEndian.PutUint16(
		bitmap[26:28],
		1,
	)

	binary.LittleEndian.PutUint16(
		bitmap[28:30],
		uint16(bitsPerPixel),
	)

	binary.LittleEndian.PutUint32(
		bitmap[34:38],
		pixelSize,
	)

	for index := pixelOffset; index < fileSize; index++ {
		bitmap[index] = byte(index)
	}

	return bitmap
}

func testBitmap32TopDown(
	t *testing.T,
	width int,
	height int,
) []byte {
	t.Helper()

	if width != 3 || height != 2 {
		t.Fatal(
			"testBitmap32TopDown() currently expects dimensions 3x2",
		)
	}

	const (
		bitsPerPixel = 32
		pixelOffset  = bitmapDataOffset
	)

	rowSize := width * 4
	pixelSize := rowSize * height
	fileSize := pixelOffset + pixelSize

	bitmap := make([]byte, fileSize)

	copy(bitmap[0:2], bitmapSignature)

	binary.LittleEndian.PutUint32(
		bitmap[2:6],
		uint32(fileSize),
	)

	binary.LittleEndian.PutUint32(
		bitmap[10:14],
		pixelOffset,
	)

	binary.LittleEndian.PutUint32(
		bitmap[14:18],
		bitmapInfoHeaderSize,
	)

	binary.LittleEndian.PutUint32(
		bitmap[18:22],
		uint32(width),
	)

	binary.LittleEndian.PutUint32(
		bitmap[22:26],
		uint32(int32(-height)),
	)

	binary.LittleEndian.PutUint16(
		bitmap[26:28],
		1,
	)

	binary.LittleEndian.PutUint16(
		bitmap[28:30],
		bitsPerPixel,
	)

	binary.LittleEndian.PutUint32(
		bitmap[34:38],
		uint32(pixelSize),
	)

	pixels := [][]color.NRGBA{
		{
			{R: 0xff, A: 0xff},
			{G: 0xff, A: 0xff},
			{B: 0xff, A: 0xff},
		},
		{
			{R: 0xff, G: 0xff, A: 0xff},
			{R: 0xff, B: 0xff, A: 0xff},
			{G: 0xff, B: 0xff, A: 0xff},
		},
	}

	for y, row := range pixels {
		for x, pixel := range row {
			offset := pixelOffset + y*rowSize + x*4

			bitmap[offset] = pixel.B
			bitmap[offset+1] = pixel.G
			bitmap[offset+2] = pixel.R
			bitmap[offset+3] = pixel.A
		}
	}

	return bitmap
}
