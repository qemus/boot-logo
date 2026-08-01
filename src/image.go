package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"

	"github.com/linuxboot/fiano/pkg/uefi"
)

const (
	bitmapFileHeaderSize = 14
	bitmapCoreHeaderSize = 12
	bitmapInfoHeaderSize = 40
)

var bitmapSignature = []byte{'B', 'M'}

func readBitmap(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read image %q: %w", path, err)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("image %q is empty", path)
	}

	location, err := parseBitmapAt(data, 0)
	if err != nil {
		return nil, fmt.Errorf("invalid bitmap %q: %w", path, err)
	}

	if location.length != len(data) {
		return nil, fmt.Errorf(
			"invalid bitmap %q: file contains %d trailing bytes",
			path,
			len(data)-location.length,
		)
	}

	return data, nil
}

func findEmbeddedBitmaps(
	data []byte,
) ([]bitmapLocation, error) {
	var locations []bitmapLocation

	for searchOffset := 0; searchOffset+1 < len(data); {
		relativeOffset := bytes.Index(
			data[searchOffset:],
			bitmapSignature,
		)

		if relativeOffset < 0 {
			break
		}

		offset := searchOffset + relativeOffset

		location, err := parseBitmapAt(data, offset)
		if err != nil {
			searchOffset = offset + len(bitmapSignature)

			continue
		}

		locations = append(locations, location)
		searchOffset = location.offset + location.length
	}

	return locations, nil
}

func parseBitmapAt(
	data []byte,
	offset int,
) (bitmapLocation, error) {
	if offset < 0 || offset > len(data)-bitmapFileHeaderSize {
		return bitmapLocation{}, fmt.Errorf(
			"bitmap header is outside the available data",
		)
	}

	bitmap := data[offset:]

	if !bytes.Equal(bitmap[:2], bitmapSignature) {
		return bitmapLocation{}, fmt.Errorf(
			"missing bitmap signature",
		)
	}

	fileSize := uint64(binary.LittleEndian.Uint32(
		bitmap[2:6],
	))

	if fileSize < bitmapFileHeaderSize+4 {
		return bitmapLocation{}, fmt.Errorf(
			"invalid file size: %d",
			fileSize,
		)
	}

	if fileSize > uint64(len(bitmap)) {
		return bitmapLocation{}, fmt.Errorf(
			"file size exceeds available data",
		)
	}

	pixelOffset := uint64(binary.LittleEndian.Uint32(
		bitmap[10:14],
	))

	dibSize := uint64(binary.LittleEndian.Uint32(
		bitmap[14:18],
	))

	if dibSize != bitmapCoreHeaderSize &&
		dibSize < bitmapInfoHeaderSize {
		return bitmapLocation{}, fmt.Errorf(
			"unsupported DIB header size: %d",
			dibSize,
		)
	}

	headerEnd := uint64(bitmapFileHeaderSize) + dibSize

	if headerEnd > fileSize {
		return bitmapLocation{}, fmt.Errorf(
			"DIB header exceeds bitmap size",
		)
	}

	var (
		width        uint64
		height       uint64
		planes       uint16
		bitsPerPixel uint16
		colorsUsed   uint64
		paletteSize  uint64
	)

	if dibSize == bitmapCoreHeaderSize {
		width = uint64(binary.LittleEndian.Uint16(
			bitmap[18:20],
		))

		height = uint64(binary.LittleEndian.Uint16(
			bitmap[20:22],
		))

		planes = binary.LittleEndian.Uint16(
			bitmap[22:24],
		)

		bitsPerPixel = binary.LittleEndian.Uint16(
			bitmap[24:26],
		)

		if bitsPerPixel <= 8 {
			colorsUsed = uint64(1) << bitsPerPixel
			paletteSize = colorsUsed * 3
		}
	} else {
		widthValue := int64(int32(binary.LittleEndian.Uint32(
			bitmap[18:22],
		)))

		heightValue := int64(int32(binary.LittleEndian.Uint32(
			bitmap[22:26],
		)))

		if widthValue <= 0 {
			return bitmapLocation{}, fmt.Errorf(
				"invalid bitmap width: %d",
				widthValue,
			)
		}

		if heightValue == 0 {
			return bitmapLocation{}, fmt.Errorf(
				"invalid bitmap height: 0",
			)
		}

		width = uint64(widthValue)

		if heightValue < 0 {
			height = uint64(-heightValue)
		} else {
			height = uint64(heightValue)
		}

		planes = binary.LittleEndian.Uint16(
			bitmap[26:28],
		)

		bitsPerPixel = binary.LittleEndian.Uint16(
			bitmap[28:30],
		)

		compression := binary.LittleEndian.Uint32(
			bitmap[30:34],
		)

		if compression != 0 {
			return bitmapLocation{}, fmt.Errorf(
				"compressed bitmaps are not supported",
			)
		}

		if dibSize >= bitmapInfoHeaderSize {
			colorsUsed = uint64(binary.LittleEndian.Uint32(
				bitmap[46:50],
			))
		}

		if bitsPerPixel <= 8 {
			maximumColors := uint64(1) << bitsPerPixel

			if colorsUsed == 0 {
				colorsUsed = maximumColors
			}

			if colorsUsed > maximumColors {
				return bitmapLocation{}, fmt.Errorf(
					"invalid palette size: %d",
					colorsUsed,
				)
			}

			paletteSize = colorsUsed * 4
		}
	}

	if width == 0 || height == 0 {
		return bitmapLocation{}, fmt.Errorf(
			"bitmap dimensions cannot be zero",
		)
	}

	if planes != 1 {
		return bitmapLocation{}, fmt.Errorf(
			"invalid plane count: %d",
			planes,
		)
	}

	switch bitsPerPixel {
	case 1, 4, 8, 16, 24, 32:
	default:
		return bitmapLocation{}, fmt.Errorf(
			"unsupported color depth: %d bits",
			bitsPerPixel,
		)
	}

	minimumPixelOffset := headerEnd + paletteSize

	if pixelOffset < minimumPixelOffset {
		return bitmapLocation{}, fmt.Errorf(
			"pixel data overlaps the bitmap header or palette",
		)
	}

	if pixelOffset >= fileSize {
		return bitmapLocation{}, fmt.Errorf(
			"pixel offset exceeds bitmap size",
		)
	}

	rowBits := width * uint64(bitsPerPixel)
	rowSize := ((rowBits + 31) / 32) * 4
	pixelSize := rowSize * height

	if pixelSize > fileSize-pixelOffset {
		return bitmapLocation{}, fmt.Errorf(
			"pixel data exceeds bitmap size",
		)
	}

	return bitmapLocation{
		offset: offset,
		length: int(fileSize),
	}, nil
}

func replaceEmbeddedBitmap(
	section *uefi.Section,
	location bitmapLocation,
	image []byte,
) error {
	if section.Header.Type != uefi.SectionTypeRaw {
		return fmt.Errorf(
			"bitmap is not stored in a raw firmware section",
		)
	}

	sectionData := section.Buf()
	headerLength := 4

	if section.Header.Size == [3]uint8{0xFF, 0xFF, 0xFF} {
		headerLength = 8
	}

	start := location.offset
	end := start + location.length

	if start < headerLength ||
		start > len(sectionData) ||
		end < start ||
		end > len(sectionData) {
		return fmt.Errorf(
			"bitmap location is outside the firmware section",
		)
	}

	payloadStart := start - headerLength
	payloadEnd := end - headerLength
	payload := sectionData[headerLength:]

	updated := make(
		[]byte,
		0,
		len(payload)-location.length+len(image),
	)

	updated = append(updated, payload[:payloadStart]...)
	updated = append(updated, image...)
	updated = append(updated, payload[payloadEnd:]...)

	section.SetBuf(updated)

	if err := section.GenSecHeader(); err != nil {
		return fmt.Errorf(
			"rebuild firmware section header: %w",
			err,
		)
	}

	section.Modified = true

	return nil
}
