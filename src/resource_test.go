package main

import (
	"encoding/binary"
	"image"
	"image/color"
	"testing"
)

func TestEncodeHIIImageUses8BitFor256Colors(t *testing.T) {
	source := image.NewNRGBA(
		image.Rect(0, 0, 16, 16),
	)

	for index := 0; index < 256; index++ {
		source.SetNRGBA(
			index%16,
			index/16,
			color.NRGBA{
				R: byte(index),
				G: byte(255 - index),
				B: byte(index ^ 0x5a),
				A: 0xff,
			},
		)
	}

	block, palette, err := encodeHIIImage(source)
	if err != nil {
		t.Fatalf("encodeHIIImage() returned an error: %v", err)
	}

	if block[0] != hiiImage8Bit {
		t.Fatalf(
			"HII image type = %#02x, want %#02x",
			block[0],
			hiiImage8Bit,
		)
	}

	if len(block) != 6+256 {
		t.Fatalf(
			"8-bit HII block length = %d, want %d",
			len(block),
			6+256,
		)
	}

	if len(palette) != 256*3 {
		t.Fatalf(
			"HII palette length = %d, want %d",
			len(palette),
			256*3,
		)
	}

	for index := 0; index < 256; index++ {
		if block[6+index] != byte(index) {
			t.Fatalf(
				"pixel %d palette index = %d, want %d",
				index,
				block[6+index],
				index,
			)
		}

		paletteOffset := index * 3
		expected := source.NRGBAAt(
			index%16,
			index/16,
		)

		if palette[paletteOffset] != expected.B ||
			palette[paletteOffset+1] != expected.G ||
			palette[paletteOffset+2] != expected.R {
			t.Fatalf(
				"palette color %d = {%d %d %d}, want {%d %d %d}",
				index,
				palette[paletteOffset+2],
				palette[paletteOffset+1],
				palette[paletteOffset],
				expected.R,
				expected.G,
				expected.B,
			)
		}
	}
}

func TestEncodeHIIImageKeeps24BitAbove256Colors(t *testing.T) {
	source := image.NewNRGBA(
		image.Rect(0, 0, 257, 1),
	)

	for index := 0; index < 257; index++ {
		source.SetNRGBA(
			index,
			0,
			color.NRGBA{
				R: byte(index),
				G: byte(index >> 8),
				A: 0xff,
			},
		)
	}

	block, palette, err := encodeHIIImage(source)
	if err != nil {
		t.Fatalf("encodeHIIImage() returned an error: %v", err)
	}

	if block[0] != hiiImage24Bit {
		t.Fatalf(
			"HII image type = %#02x, want %#02x",
			block[0],
			hiiImage24Bit,
		)
	}

	if palette != nil {
		t.Fatalf(
			"24-bit HII image returned a %d-byte palette",
			len(palette),
		)
	}
}

func TestReplaceHIIImageAddsExactPalette(t *testing.T) {
	const (
		rawOffset      = 0x200
		rawSize        = 0x800
		resourceOffset = 0x300
		resourceSize   = 0x100
		packageOffset  = resourceOffset + hiiPackageListHeaderSize
		imageOffset    = packageOffset + hiiImagePackageHeaderSize
	)

	data := make([]byte, rawOffset+rawSize)

	oldBlock := []byte{
		hiiImage24Bit,
		0x02, 0x00,
		0x01, 0x00,
		0x00, 0x00, 0xff,
		0x00, 0xff, 0x00,
	}

	oldPackageLength :=
		hiiImagePackageHeaderSize +
			len(oldBlock) +
			1

	packageHeader :=
		uint32(hiiPackageImages)<<24 |
			uint32(oldPackageLength)

	binary.LittleEndian.PutUint32(
		data[packageOffset:packageOffset+4],
		packageHeader,
	)
	binary.LittleEndian.PutUint32(
		data[packageOffset+4:packageOffset+8],
		hiiImagePackageHeaderSize,
	)

	copy(data[imageOffset:], oldBlock)
	data[imageOffset+len(oldBlock)] = hiiImageEnd

	endPackageOffset := packageOffset + oldPackageLength
	binary.LittleEndian.PutUint32(
		data[endPackageOffset:endPackageOffset+4],
		uint32(hiiPackageEnd)<<24|hiiPackageHeaderSize,
	)

	listLength :=
		hiiPackageListHeaderSize +
			oldPackageLength +
			hiiPackageHeaderSize

	binary.LittleEndian.PutUint32(
		data[resourceOffset+16:resourceOffset+20],
		uint32(listLength),
	)

	resource := peResourceInfo{
		sizeOfInitializedDataOffset: 0x40,
		sizeOfImageOffset:           0x44,
		checksumOffset:              0x48,
		directorySizeOffset:         0x4c,
		fileAlignment:               0x200,
		sectionAlignment:            0x1000,
		directorySize:               resourceSize,
		section: peSectionInfo{
			headerOffset:   0x80,
			virtualSize:    rawSize,
			virtualAddress: 0x1000,
			rawSize:        rawSize,
			rawOffset:      rawOffset,
		},
		dataEntryOffset: 0x50,
		dataOffset:      resourceOffset,
		dataSize:        resourceSize,
	}
	resource.sections = []peSectionInfo{
		resource.section,
	}

	binary.LittleEndian.PutUint32(
		data[resource.sizeOfInitializedDataOffset:resource.sizeOfInitializedDataOffset+4],
		rawSize,
	)
	binary.LittleEndian.PutUint32(
		data[resource.directorySizeOffset:resource.directorySizeOffset+4],
		resourceSize,
	)

	location := hiiImageLocation{
		resource:          resource,
		packageListLength: listLength,
		packageOffset:     packageOffset,
		packageLength:     oldPackageLength,
		imageInfoOffset:   hiiImagePackageHeaderSize,
		blockOffset:       imageOffset,
		blockLength:       len(oldBlock),
		blockType:         hiiImage24Bit,
		width:             2,
		height:            1,
	}

	source := image.NewNRGBA(
		image.Rect(0, 0, 2, 1),
	)
	source.SetNRGBA(
		0,
		0,
		color.NRGBA{
			R: 0x12,
			G: 0x34,
			B: 0x56,
			A: 0xff,
		},
	)
	source.SetNRGBA(
		1,
		0,
		color.NRGBA{
			R: 0xab,
			G: 0xcd,
			B: 0xef,
			A: 0xff,
		},
	)

	updated, err := replaceHIIImage(
		data,
		location,
		source,
	)
	if err != nil {
		t.Fatalf("replaceHIIImage() returned an error: %v", err)
	}

	newBlockLength := 6 + 2
	newPaletteOffset :=
		hiiImagePackageHeaderSize +
			newBlockLength +
			1
	newPackageLength :=
		newPaletteOffset +
			2 +
			2 +
			6

	updatedPackageHeader := binary.LittleEndian.Uint32(
		updated[packageOffset : packageOffset+4],
	)

	if int(updatedPackageHeader&maxUint24Value) != newPackageLength {
		t.Fatalf(
			"updated package length = %d, want %d",
			updatedPackageHeader&maxUint24Value,
			newPackageLength,
		)
	}

	actualPaletteOffset := int(binary.LittleEndian.Uint32(
		updated[packageOffset+8 : packageOffset+12],
	))

	if actualPaletteOffset != newPaletteOffset {
		t.Fatalf(
			"palette offset = %d, want %d",
			actualPaletteOffset,
			newPaletteOffset,
		)
	}

	updatedLocation := location
	updatedLocation.packageLength = newPackageLength
	updatedLocation.paletteInfoOffset = newPaletteOffset
	updatedLocation.blockLength = newBlockLength
	updatedLocation.blockType = hiiImage8Bit
	updatedLocation.palette = 1

	actual, err := decodeHIIImage(
		updated,
		updatedLocation,
	)
	if err != nil {
		t.Fatalf("decodeHIIImage() returned an error: %v", err)
	}

	for x := 0; x < 2; x++ {
		got := color.NRGBAModel.Convert(
			actual.At(x, 0),
		).(color.NRGBA)
		want := source.NRGBAAt(x, 0)

		if got != want {
			t.Fatalf(
				"pixel %d = %#v, want %#v",
				x,
				got,
				want,
			)
		}
	}
}
