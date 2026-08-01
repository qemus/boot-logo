package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"

	"github.com/linuxboot/fiano/pkg/uefi"
)

const (
	bitmapFileHeaderSize = 14
	bitmapCoreHeaderSize = 12
	bitmapInfoHeaderSize = 40
	bitmapDataOffset     = bitmapFileHeaderSize +
		bitmapInfoHeaderSize

	maxBitmapDimension = int64(1<<31 - 1)
	maxBitmapFileSize  = uint64(1<<32 - 1)
)

type bitmapMetadata struct {
	location         bitmapLocation
	width            int
	height           int
	topDown          bool
	bitsPerPixel     uint16
	pixelOffset      int
	rowSize          int
	paletteOffset    int
	paletteEntries   int
	paletteEntrySize int
}

var (
	bitmapSignature = []byte{'B', 'M'}

	pngSignature = []byte{
		0x89,
		0x50,
		0x4e,
		0x47,
		0x0d,
		0x0a,
		0x1a,
		0x0a,
	}

	jpegSignature = []byte{
		0xff,
		0xd8,
	}
)

func readBitmap(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read image %q: %w", path, err)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("image %q is empty", path)
	}

	switch {
	case bytes.HasPrefix(data, bitmapSignature):
		metadata, err := inspectBitmapAt(data, 0)
		if err != nil {
			return nil, fmt.Errorf(
				"invalid bitmap %q: %w",
				path,
				err,
			)
		}

		if metadata.location.length != len(data) {
			return nil, fmt.Errorf(
				"invalid bitmap %q: file contains %d trailing bytes",
				path,
				len(data)-metadata.location.length,
			)
		}

		source, err := decodeBitmap(data)
		if err != nil {
			return nil, fmt.Errorf(
				"decode bitmap %q: %w",
				path,
				err,
			)
		}

		bitmap, err := encodeBitmap(source)
		if err != nil {
			return nil, fmt.Errorf(
				"convert bitmap %q: %w",
				path,
				err,
			)
		}

		return bitmap, nil

	case bytes.HasPrefix(data, pngSignature):
		source, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf(
				"decode PNG image %q: %w",
				path,
				err,
			)
		}

		bitmap, err := encodeBitmap(source)
		if err != nil {
			return nil, fmt.Errorf(
				"convert PNG image %q: %w",
				path,
				err,
			)
		}

		return bitmap, nil

	case bytes.HasPrefix(data, jpegSignature):
		source, err := jpeg.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf(
				"decode JPEG image %q: %w",
				path,
				err,
			)
		}

		bitmap, err := encodeBitmap(source)
		if err != nil {
			return nil, fmt.Errorf(
				"convert JPEG image %q: %w",
				path,
				err,
			)
		}

		return bitmap, nil

	default:
		return nil, fmt.Errorf(
			"unsupported image format %q: expected BMP, PNG or JPEG",
			path,
		)
	}
}

func encodeBitmap(source image.Image) ([]byte, error) {
	if source == nil {
		return nil, fmt.Errorf("image is nil")
	}

	bounds := source.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf(
			"image dimensions must be greater than zero",
		)
	}

	if int64(width) > maxBitmapDimension ||
		int64(height) > maxBitmapDimension {
		return nil, fmt.Errorf(
			"image dimensions are too large: %dx%d",
			width,
			height,
		)
	}

	width64 := uint64(width)
	height64 := uint64(height)

	rowSize := ((width64*3 + 3) / 4) * 4
	pixelSize := rowSize * height64
	fileSize := uint64(bitmapDataOffset) + pixelSize

	if fileSize > maxBitmapFileSize {
		return nil, fmt.Errorf(
			"converted bitmap is too large: %d bytes",
			fileSize,
		)
	}

	maximumInt := uint64(^uint(0) >> 1)

	if fileSize > maximumInt {
		return nil, fmt.Errorf(
			"converted bitmap exceeds the platform size limit",
		)
	}

	bitmap := make([]byte, int(fileSize))

	copy(bitmap[0:2], bitmapSignature)

	binary.LittleEndian.PutUint32(
		bitmap[2:6],
		uint32(fileSize),
	)

	binary.LittleEndian.PutUint32(
		bitmap[10:14],
		bitmapDataOffset,
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
		uint32(height),
	)

	binary.LittleEndian.PutUint16(
		bitmap[26:28],
		1,
	)

	binary.LittleEndian.PutUint16(
		bitmap[28:30],
		24,
	)

	binary.LittleEndian.PutUint32(
		bitmap[34:38],
		uint32(pixelSize),
	)

	rowLength := int(rowSize)

	for destinationY := 0; destinationY < height; destinationY++ {
		sourceY := bounds.Min.Y + height - destinationY - 1

		rowOffset := bitmapDataOffset +
			destinationY*rowLength

		for x := 0; x < width; x++ {
			sourceX := bounds.Min.X + x

			red, green, blue, _ := source.At(
				sourceX,
				sourceY,
			).RGBA()

			pixelOffset := rowOffset + x*3

			bitmap[pixelOffset] = byte(blue >> 8)
			bitmap[pixelOffset+1] = byte(green >> 8)
			bitmap[pixelOffset+2] = byte(red >> 8)
		}
	}

	return bitmap, nil
}

func decodeBitmap(data []byte) (image.Image, error) {
	metadata, err := inspectBitmapAt(data, 0)
	if err != nil {
		return nil, err
	}

	if metadata.location.length != len(data) {
		return nil, fmt.Errorf(
			"bitmap contains trailing data",
		)
	}

	palette, err := decodeBitmapPalette(data, metadata)
	if err != nil {
		return nil, err
	}

	output := image.NewNRGBA(
		image.Rect(
			0,
			0,
			metadata.width,
			metadata.height,
		),
	)

	for y := 0; y < metadata.height; y++ {
		sourceRow := y

		if !metadata.topDown {
			sourceRow = metadata.height - 1 - y
		}

		rowOffset := metadata.pixelOffset +
			sourceRow*metadata.rowSize

		row := data[rowOffset : rowOffset+metadata.rowSize]

		for x := 0; x < metadata.width; x++ {
			pixel, err := decodeBitmapPixel(
				row,
				x,
				metadata,
				palette,
			)
			if err != nil {
				return nil, err
			}

			output.Set(x, y, pixel)
		}
	}

	return output, nil
}

func decodeBitmapPalette(
	data []byte,
	metadata bitmapMetadata,
) ([]color.NRGBA, error) {
	if metadata.paletteEntries == 0 {
		return nil, nil
	}

	palette := make(
		[]color.NRGBA,
		metadata.paletteEntries,
	)

	for index := 0; index < metadata.paletteEntries; index++ {
		offset := metadata.paletteOffset +
			index*metadata.paletteEntrySize

		switch metadata.paletteEntrySize {
		case 3:
			palette[index] = color.NRGBA{
				B: data[offset],
				G: data[offset+1],
				R: data[offset+2],
				A: 0xFF,
			}

		case 4:
			palette[index] = color.NRGBA{
				B: data[offset],
				G: data[offset+1],
				R: data[offset+2],
				A: 0xFF,
			}

		default:
			return nil, fmt.Errorf(
				"unsupported palette entry size: %d",
				metadata.paletteEntrySize,
			)
		}
	}

	return palette, nil
}

func decodeBitmapPixel(
	row []byte,
	x int,
	metadata bitmapMetadata,
	palette []color.NRGBA,
) (color.NRGBA, error) {
	switch metadata.bitsPerPixel {
	case 1:
		byteIndex := x / 8
		bitIndex := 7 - (x % 8)
		paletteIndex := int(
			(row[byteIndex] >> bitIndex) & 0x01,
		)

		if paletteIndex >= len(palette) {
			return color.NRGBA{}, fmt.Errorf(
				"bitmap palette index %d is out of range",
				paletteIndex,
			)
		}

		return palette[paletteIndex], nil

	case 4:
		byteIndex := x / 2
		value := row[byteIndex]

		var paletteIndex int

		if x%2 == 0 {
			paletteIndex = int((value >> 4) & 0x0F)
		} else {
			paletteIndex = int(value & 0x0F)
		}

		if paletteIndex >= len(palette) {
			return color.NRGBA{}, fmt.Errorf(
				"bitmap palette index %d is out of range",
				paletteIndex,
			)
		}

		return palette[paletteIndex], nil

	case 8:
		paletteIndex := int(row[x])

		if paletteIndex >= len(palette) {
			return color.NRGBA{}, fmt.Errorf(
				"bitmap palette index %d is out of range",
				paletteIndex,
			)
		}

		return palette[paletteIndex], nil

	case 16:
		offset := x * 2
		value := binary.LittleEndian.Uint16(
			row[offset : offset+2],
		)

		red := uint8((value >> 10) & 0x1F)
		green := uint8((value >> 5) & 0x1F)
		blue := uint8(value & 0x1F)

		return color.NRGBA{
			R: (red << 3) | (red >> 2),
			G: (green << 3) | (green >> 2),
			B: (blue << 3) | (blue >> 2),
			A: 0xFF,
		}, nil

	case 24:
		offset := x * 3

		return color.NRGBA{
			B: row[offset],
			G: row[offset+1],
			R: row[offset+2],
			A: 0xFF,
		}, nil

	case 32:
		offset := x * 4

		return color.NRGBA{
			B: row[offset],
			G: row[offset+1],
			R: row[offset+2],
			A: 0xFF,
		}, nil

	default:
		return color.NRGBA{}, fmt.Errorf(
			"unsupported bitmap color depth: %d bits",
			metadata.bitsPerPixel,
		)
	}
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
	metadata, err := inspectBitmapAt(data, offset)
	if err != nil {
		return bitmapLocation{}, err
	}

	return metadata.location, nil
}

func inspectBitmapAt(
	data []byte,
	offset int,
) (bitmapMetadata, error) {
	if offset < 0 || offset > len(data) {
		return bitmapMetadata{}, fmt.Errorf(
			"bitmap offset is outside the available data",
		)
	}

	if len(data)-offset < bitmapFileHeaderSize+4 {
		return bitmapMetadata{}, fmt.Errorf(
			"bitmap header is incomplete",
		)
	}

	bitmap := data[offset:]

	if !bytes.Equal(bitmap[:2], bitmapSignature) {
		return bitmapMetadata{}, fmt.Errorf(
			"missing bitmap signature",
		)
	}

	fileSize := uint64(binary.LittleEndian.Uint32(
		bitmap[2:6],
	))

	if fileSize < bitmapFileHeaderSize+4 {
		return bitmapMetadata{}, fmt.Errorf(
			"invalid file size: %d",
			fileSize,
		)
	}

	if fileSize > uint64(len(bitmap)) {
		return bitmapMetadata{}, fmt.Errorf(
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
		return bitmapMetadata{}, fmt.Errorf(
			"unsupported DIB header size: %d",
			dibSize,
		)
	}

	headerEnd := uint64(bitmapFileHeaderSize) + dibSize

	if headerEnd > fileSize {
		return bitmapMetadata{}, fmt.Errorf(
			"DIB header exceeds bitmap size",
		)
	}

	var (
		width            uint64
		height           uint64
		topDown          bool
		planes           uint16
		bitsPerPixel     uint16
		colorsUsed       uint64
		paletteSize      uint64
		paletteEntrySize int
	)

	if dibSize == bitmapCoreHeaderSize {
		if headerEnd < 26 {
			return bitmapMetadata{}, fmt.Errorf(
				"bitmap core header is incomplete",
			)
		}

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

		paletteEntrySize = 3

		if bitsPerPixel <= 8 {
			colorsUsed = uint64(1) << bitsPerPixel
			paletteSize = colorsUsed * uint64(paletteEntrySize)
		}
	} else {
		if headerEnd < bitmapDataOffset {
			return bitmapMetadata{}, fmt.Errorf(
				"bitmap information header is incomplete",
			)
		}

		widthValue := int64(int32(
			binary.LittleEndian.Uint32(
				bitmap[18:22],
			),
		))

		heightValue := int64(int32(
			binary.LittleEndian.Uint32(
				bitmap[22:26],
			),
		))

		if widthValue <= 0 {
			return bitmapMetadata{}, fmt.Errorf(
				"invalid bitmap width: %d",
				widthValue,
			)
		}

		if heightValue == 0 {
			return bitmapMetadata{}, fmt.Errorf(
				"invalid bitmap height: 0",
			)
		}

		width = uint64(widthValue)

		if heightValue < 0 {
			height = uint64(-heightValue)
			topDown = true
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
			return bitmapMetadata{}, fmt.Errorf(
				"compressed bitmaps are not supported",
			)
		}

		paletteEntrySize = 4
		colorsUsed = uint64(binary.LittleEndian.Uint32(
			bitmap[46:50],
		))

		if bitsPerPixel <= 8 {
			maximumColors := uint64(1) << bitsPerPixel

			if colorsUsed == 0 {
				colorsUsed = maximumColors
			}

			if colorsUsed > maximumColors {
				return bitmapMetadata{}, fmt.Errorf(
					"invalid palette size: %d",
					colorsUsed,
				)
			}

			paletteSize = colorsUsed * uint64(paletteEntrySize)
		}
	}

	if width == 0 || height == 0 {
		return bitmapMetadata{}, fmt.Errorf(
			"bitmap dimensions cannot be zero",
		)
	}

	if planes != 1 {
		return bitmapMetadata{}, fmt.Errorf(
			"invalid plane count: %d",
			planes,
		)
	}

	switch bitsPerPixel {
	case 1, 4, 8, 16, 24, 32:
	default:
		return bitmapMetadata{}, fmt.Errorf(
			"unsupported color depth: %d bits",
			bitsPerPixel,
		)
	}

	minimumPixelOffset := headerEnd + paletteSize

	if pixelOffset < minimumPixelOffset {
		return bitmapMetadata{}, fmt.Errorf(
			"pixel data overlaps the bitmap header or palette",
		)
	}

	if pixelOffset >= fileSize {
		return bitmapMetadata{}, fmt.Errorf(
			"pixel offset exceeds bitmap size",
		)
	}

	rowBits := width * uint64(bitsPerPixel)
	rowSize := ((rowBits + 31) / 32) * 4

	if rowSize == 0 {
		return bitmapMetadata{}, fmt.Errorf(
			"invalid bitmap row size",
		)
	}

	availablePixelData := fileSize - pixelOffset

	if height > availablePixelData/rowSize {
		return bitmapMetadata{}, fmt.Errorf(
			"pixel data exceeds bitmap size",
		)
	}

	if width > uint64(maxBitmapDimension) ||
		height > uint64(maxBitmapDimension) {
		return bitmapMetadata{}, fmt.Errorf(
			"bitmap dimensions are too large",
		)
	}

	return bitmapMetadata{
		location: bitmapLocation{
			offset: offset,
			length: int(fileSize),
		},
		width:            int(width),
		height:           int(height),
		topDown:          topDown,
		bitsPerPixel:     bitsPerPixel,
		pixelOffset:      int(pixelOffset),
		rowSize:          int(rowSize),
		paletteOffset:    bitmapFileHeaderSize + int(dibSize),
		paletteEntries:   int(colorsUsed),
		paletteEntrySize: paletteEntrySize,
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
