package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
)

const (
	peDOSSignature           = 0x5a4d
	peSignature              = 0x00004550
	peOptionalHeader32Magic  = 0x010b
	peOptionalHeader64Magic  = 0x020b
	peFileHeaderSize         = 20
	peSectionHeaderSize      = 40
	peResourceDirectoryIndex = 2

	peResourceDirectoryHeaderSize = 16
	peResourceDirectoryEntrySize  = 8
	peResourceDataEntrySize       = 16

	hiiPackageListHeaderSize  = 20
	hiiPackageHeaderSize      = 4
	hiiImagePackageHeaderSize = 12

	hiiPackageImages = 0x06
	hiiPackageEnd    = 0xdf

	hiiImageEnd        = 0x00
	hiiImage1Bit       = 0x10
	hiiImage1BitTrans  = 0x11
	hiiImage4Bit       = 0x12
	hiiImage4BitTrans  = 0x13
	hiiImage8Bit       = 0x14
	hiiImage8BitTrans  = 0x15
	hiiImage24Bit      = 0x16
	hiiImage24BitTrans = 0x17
	hiiImageJPEG       = 0x18
	hiiImagePNG        = 0x19
	hiiImageDuplicate  = 0x20
	hiiImageSkip2      = 0x21
	hiiImageSkip1      = 0x22
	hiiImageExt1       = 0x30
	hiiImageExt2       = 0x31
	hiiImageExt4       = 0x32

	maxUint8Value  = 1<<8 - 1
	maxUint16Value = 1<<16 - 1
	maxUint24Value = 1<<24 - 1
	maxUint32Value = 1<<32 - 1
)

var errHIIResourceNotFound = errors.New(
	"HII resource was not found",
)

type peSectionInfo struct {
	headerOffset   int
	virtualSize    uint32
	virtualAddress uint32
	rawSize        uint32
	rawOffset      uint32
}

type peResourceInfo struct {
	sizeOfInitializedDataOffset int
	sizeOfImageOffset           int
	checksumOffset              int
	directorySizeOffset         int

	fileAlignment    uint32
	sectionAlignment uint32
	directorySize    uint32

	section  peSectionInfo
	sections []peSectionInfo

	dataEntryOffset int
	dataOffset      int
	dataSize        int
}

type hiiImageLocation struct {
	resource peResourceInfo

	packageListLength int

	packageOffset int
	packageLength int

	imageInfoOffset   int
	paletteInfoOffset int

	blockOffset int
	blockLength int
	blockType   byte
	palette     byte

	width  int
	height int
}

type hiiImageBlockInfo struct {
	length       int
	width        int
	height       int
	concrete     bool
	paletteIndex byte
}

func findHIIImages(
	data []byte,
) ([]hiiImageLocation, error) {
	resource, err := findHIIResource(data)
	if errors.Is(err, errHIIResourceNotFound) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	resourceStart := resource.dataOffset
	resourceEnd := resourceStart + resource.dataSize

	if resourceStart < 0 ||
		resourceEnd > len(data) ||
		resource.dataSize < hiiPackageListHeaderSize {
		return nil, fmt.Errorf(
			"HII package list header is incomplete",
		)
	}

	listLength := int(binary.LittleEndian.Uint32(
		data[resourceStart+16 : resourceStart+20],
	))

	if listLength < hiiPackageListHeaderSize {
		return nil, fmt.Errorf(
			"invalid HII package list length: %d",
			listLength,
		)
	}

	listEnd := resourceStart + listLength

	if listEnd > resourceEnd {
		return nil, fmt.Errorf(
			"HII package list length %d exceeds resource size %d",
			listLength,
			resource.dataSize,
		)
	}

	var images []hiiImageLocation

	for offset := resourceStart + hiiPackageListHeaderSize; offset < listEnd; {
		if listEnd-offset < hiiPackageHeaderSize {
			return nil, fmt.Errorf(
				"HII package header is incomplete",
			)
		}

		header := binary.LittleEndian.Uint32(
			data[offset : offset+hiiPackageHeaderSize],
		)

		packageLength := int(header & maxUint24Value)
		packageType := byte(header >> 24)

		if packageLength < hiiPackageHeaderSize {
			return nil, fmt.Errorf(
				"invalid HII package length: %d",
				packageLength,
			)
		}

		packageEnd := offset + packageLength

		if packageEnd > listEnd {
			return nil, fmt.Errorf(
				"HII package exceeds its package list",
			)
		}

		switch packageType {
		case hiiPackageImages:
			packageImages, err := parseHIIImagePackage(
				data,
				resource,
				listLength,
				offset,
				packageLength,
			)
			if err != nil {
				return nil, err
			}

			images = append(
				images,
				packageImages...,
			)

		case hiiPackageEnd:
			return images, nil
		}

		offset = packageEnd
	}

	return images, nil
}

func parseHIIImagePackage(
	data []byte,
	resource peResourceInfo,
	listLength int,
	packageOffset int,
	packageLength int,
) ([]hiiImageLocation, error) {
	if packageLength < hiiImagePackageHeaderSize {
		return nil, fmt.Errorf(
			"HII image package header is incomplete",
		)
	}

	packageEnd := packageOffset + packageLength

	if packageOffset < 0 || packageEnd > len(data) {
		return nil, fmt.Errorf(
			"HII image package is outside the PE image",
		)
	}

	imageInfoOffset := int(binary.LittleEndian.Uint32(
		data[packageOffset+4 : packageOffset+8],
	))

	paletteInfoOffset := int(binary.LittleEndian.Uint32(
		data[packageOffset+8 : packageOffset+12],
	))

	if imageInfoOffset < hiiImagePackageHeaderSize ||
		imageInfoOffset >= packageLength {
		return nil, fmt.Errorf(
			"invalid HII image information offset: %d",
			imageInfoOffset,
		)
	}

	if paletteInfoOffset < 0 ||
		paletteInfoOffset >= packageLength {
		if paletteInfoOffset != 0 {
			return nil, fmt.Errorf(
				"invalid HII palette information offset: %d",
				paletteInfoOffset,
			)
		}
	}

	var images []hiiImageLocation

	for offset := packageOffset + imageInfoOffset; offset < packageEnd; {
		blockType := data[offset]

		if blockType == hiiImageEnd {
			return images, nil
		}

		info, err := parseHIIImageBlock(
			data,
			offset,
			packageEnd,
		)
		if err != nil {
			return nil, err
		}

		if info.concrete {
			images = append(
				images,
				hiiImageLocation{
					resource: resource,

					packageListLength: listLength,

					packageOffset: packageOffset,
					packageLength: packageLength,

					imageInfoOffset:   imageInfoOffset,
					paletteInfoOffset: paletteInfoOffset,

					blockOffset: offset,
					blockLength: info.length,
					blockType:   blockType,
					palette:     info.paletteIndex,

					width:  info.width,
					height: info.height,
				},
			)
		}

		offset += info.length
	}

	return nil, fmt.Errorf(
		"HII image package does not contain an end block",
	)
}

func parseHIIImageBlock(
	data []byte,
	offset int,
	limit int,
) (hiiImageBlockInfo, error) {
	if offset < 0 ||
		offset >= limit ||
		limit > len(data) {
		return hiiImageBlockInfo{}, fmt.Errorf(
			"HII image block offset is outside the package",
		)
	}

	blockType := data[offset]

	switch blockType {
	case hiiImage1Bit, hiiImage1BitTrans:
		return parseHIIPaletteImageBlock(
			data,
			offset,
			limit,
			1,
		)

	case hiiImage4Bit, hiiImage4BitTrans:
		return parseHIIPaletteImageBlock(
			data,
			offset,
			limit,
			4,
		)

	case hiiImage8Bit, hiiImage8BitTrans:
		return parseHIIPaletteImageBlock(
			data,
			offset,
			limit,
			8,
		)

	case hiiImage24Bit, hiiImage24BitTrans:
		if limit-offset < 5 {
			return hiiImageBlockInfo{}, fmt.Errorf(
				"24-bit HII image block is incomplete",
			)
		}

		width := int(binary.LittleEndian.Uint16(
			data[offset+1 : offset+3],
		))

		height := int(binary.LittleEndian.Uint16(
			data[offset+3 : offset+5],
		))

		pixelLength, err := checkedImageDataLength(
			width,
			height,
			24,
		)
		if err != nil {
			return hiiImageBlockInfo{}, err
		}

		blockLength := 5 + pixelLength

		if err := checkBlockRange(
			offset,
			blockLength,
			limit,
		); err != nil {
			return hiiImageBlockInfo{}, err
		}

		return hiiImageBlockInfo{
			length:   blockLength,
			width:    width,
			height:   height,
			concrete: true,
		}, nil

	case hiiImageJPEG, hiiImagePNG:
		if limit-offset < 5 {
			return hiiImageBlockInfo{}, fmt.Errorf(
				"compressed HII image block is incomplete",
			)
		}

		payloadLength := uint64(binary.LittleEndian.Uint32(
			data[offset+1 : offset+5],
		))

		blockLength64 := uint64(5) + payloadLength

		if blockLength64 > uint64(maxIntValue()) {
			return hiiImageBlockInfo{}, fmt.Errorf(
				"compressed HII image is too large",
			)
		}

		blockLength := int(blockLength64)

		if err := checkBlockRange(
			offset,
			blockLength,
			limit,
		); err != nil {
			return hiiImageBlockInfo{}, err
		}

		return hiiImageBlockInfo{
			length:   blockLength,
			concrete: true,
		}, nil

	case hiiImageDuplicate:
		const blockLength = 3

		if err := checkBlockRange(
			offset,
			blockLength,
			limit,
		); err != nil {
			return hiiImageBlockInfo{}, err
		}

		return hiiImageBlockInfo{
			length: blockLength,
		}, nil

	case hiiImageSkip1:
		const blockLength = 2

		if err := checkBlockRange(
			offset,
			blockLength,
			limit,
		); err != nil {
			return hiiImageBlockInfo{}, err
		}

		return hiiImageBlockInfo{
			length: blockLength,
		}, nil

	case hiiImageSkip2:
		const blockLength = 3

		if err := checkBlockRange(
			offset,
			blockLength,
			limit,
		); err != nil {
			return hiiImageBlockInfo{}, err
		}

		return hiiImageBlockInfo{
			length: blockLength,
		}, nil

	case hiiImageExt1:
		if limit-offset < 3 {
			return hiiImageBlockInfo{}, fmt.Errorf(
				"HII EXT1 image block is incomplete",
			)
		}

		blockLength := int(data[offset+2])

		if blockLength < 3 {
			return hiiImageBlockInfo{}, fmt.Errorf(
				"invalid HII EXT1 block length: %d",
				blockLength,
			)
		}

		if err := checkBlockRange(
			offset,
			blockLength,
			limit,
		); err != nil {
			return hiiImageBlockInfo{}, err
		}

		return hiiImageBlockInfo{
			length: blockLength,
		}, nil

	case hiiImageExt2:
		if limit-offset < 4 {
			return hiiImageBlockInfo{}, fmt.Errorf(
				"HII EXT2 image block is incomplete",
			)
		}

		blockLength := int(binary.LittleEndian.Uint16(
			data[offset+2 : offset+4],
		))

		if blockLength < 4 {
			return hiiImageBlockInfo{}, fmt.Errorf(
				"invalid HII EXT2 block length: %d",
				blockLength,
			)
		}

		if err := checkBlockRange(
			offset,
			blockLength,
			limit,
		); err != nil {
			return hiiImageBlockInfo{}, err
		}

		return hiiImageBlockInfo{
			length: blockLength,
		}, nil

	case hiiImageExt4:
		if limit-offset < 6 {
			return hiiImageBlockInfo{}, fmt.Errorf(
				"HII EXT4 image block is incomplete",
			)
		}

		blockLength64 := uint64(binary.LittleEndian.Uint32(
			data[offset+2 : offset+6],
		))

		if blockLength64 < 6 ||
			blockLength64 > uint64(maxIntValue()) {
			return hiiImageBlockInfo{}, fmt.Errorf(
				"invalid HII EXT4 block length: %d",
				blockLength64,
			)
		}

		blockLength := int(blockLength64)

		if err := checkBlockRange(
			offset,
			blockLength,
			limit,
		); err != nil {
			return hiiImageBlockInfo{}, err
		}

		return hiiImageBlockInfo{
			length: blockLength,
		}, nil

	default:
		return hiiImageBlockInfo{}, fmt.Errorf(
			"unsupported HII image block type: %#02x",
			blockType,
		)
	}
}

func parseHIIPaletteImageBlock(
	data []byte,
	offset int,
	limit int,
	bitsPerPixel uint64,
) (hiiImageBlockInfo, error) {
	if limit-offset < 6 {
		return hiiImageBlockInfo{}, fmt.Errorf(
			"palette-based HII image block is incomplete",
		)
	}

	paletteIndex := data[offset+1]

	width := int(binary.LittleEndian.Uint16(
		data[offset+2 : offset+4],
	))

	height := int(binary.LittleEndian.Uint16(
		data[offset+4 : offset+6],
	))

	pixelLength, err := checkedImageDataLength(
		width,
		height,
		bitsPerPixel,
	)
	if err != nil {
		return hiiImageBlockInfo{}, err
	}

	blockLength := 6 + pixelLength

	if err := checkBlockRange(
		offset,
		blockLength,
		limit,
	); err != nil {
		return hiiImageBlockInfo{}, err
	}

	return hiiImageBlockInfo{
		length:       blockLength,
		width:        width,
		height:       height,
		concrete:     true,
		paletteIndex: paletteIndex,
	}, nil
}

func decodeHIIImage(
	data []byte,
	location hiiImageLocation,
) (image.Image, error) {
	start := location.blockOffset
	end := start + location.blockLength

	if start < 0 ||
		end > len(data) ||
		start >= end {
		return nil, fmt.Errorf(
			"HII image block is outside the PE image",
		)
	}

	block := data[start:end]

	switch location.blockType {
	case hiiImage1Bit, hiiImage1BitTrans:
		return decodeHIIPaletteImage(
			data,
			location,
			block,
			1,
		)

	case hiiImage4Bit, hiiImage4BitTrans:
		return decodeHIIPaletteImage(
			data,
			location,
			block,
			4,
		)

	case hiiImage8Bit, hiiImage8BitTrans:
		return decodeHIIPaletteImage(
			data,
			location,
			block,
			8,
		)

	case hiiImage24Bit, hiiImage24BitTrans:
		return decodeHII24BitImage(block)

	case hiiImageJPEG:
		if len(block) < 5 {
			return nil, fmt.Errorf(
				"JPEG HII image block is incomplete",
			)
		}

		size := int(binary.LittleEndian.Uint32(
			block[1:5],
		))

		if size != len(block)-5 {
			return nil, fmt.Errorf(
				"JPEG HII image block has an invalid size",
			)
		}

		return jpeg.Decode(
			bytes.NewReader(block[5:]),
		)

	case hiiImagePNG:
		if len(block) < 5 {
			return nil, fmt.Errorf(
				"PNG HII image block is incomplete",
			)
		}

		size := int(binary.LittleEndian.Uint32(
			block[1:5],
		))

		if size != len(block)-5 {
			return nil, fmt.Errorf(
				"PNG HII image block has an invalid size",
			)
		}

		return png.Decode(
			bytes.NewReader(block[5:]),
		)

	default:
		return nil, fmt.Errorf(
			"unsupported HII image block type: %#02x",
			location.blockType,
		)
	}
}

func decodeHII24BitImage(
	block []byte,
) (image.Image, error) {
	if len(block) < 5 {
		return nil, fmt.Errorf(
			"24-bit HII image block is incomplete",
		)
	}

	width := int(binary.LittleEndian.Uint16(
		block[1:3],
	))

	height := int(binary.LittleEndian.Uint16(
		block[3:5],
	))

	pixelLength, err := checkedImageDataLength(
		width,
		height,
		24,
	)
	if err != nil {
		return nil, err
	}

	if len(block) != 5+pixelLength {
		return nil, fmt.Errorf(
			"24-bit HII image block has an invalid size",
		)
	}

	output := image.NewNRGBA(
		image.Rect(0, 0, width, height),
	)

	offset := 5

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			output.SetNRGBA(
				x,
				y,
				color.NRGBA{
					B: block[offset],
					G: block[offset+1],
					R: block[offset+2],
					A: 0xff,
				},
			)

			offset += 3
		}
	}

	return output, nil
}

func decodeHIIPaletteImage(
	data []byte,
	location hiiImageLocation,
	block []byte,
	bitsPerPixel uint64,
) (image.Image, error) {
	if len(block) < 6 {
		return nil, fmt.Errorf(
			"palette-based HII image block is incomplete",
		)
	}

	width := int(binary.LittleEndian.Uint16(
		block[2:4],
	))

	height := int(binary.LittleEndian.Uint16(
		block[4:6],
	))

	pixelLength, err := checkedImageDataLength(
		width,
		height,
		bitsPerPixel,
	)
	if err != nil {
		return nil, err
	}

	if len(block) != 6+pixelLength {
		return nil, fmt.Errorf(
			"palette-based HII image block has an invalid size",
		)
	}

	palette, err := readHIIPalette(
		data,
		location,
	)
	if err != nil {
		return nil, err
	}

	output := image.NewNRGBA(
		image.Rect(0, 0, width, height),
	)

	pixels := block[6:]
	rowBytes := (width*int(bitsPerPixel) + 7) / 8

	for y := 0; y < height; y++ {
		row := pixels[y*rowBytes : (y+1)*rowBytes]

		for x := 0; x < width; x++ {
			var paletteIndex int

			switch bitsPerPixel {
			case 1:
				byteIndex := x / 8
				bitIndex := 7 - x%8

				paletteIndex = int(
					(row[byteIndex] >> bitIndex) & 0x01,
				)

			case 4:
				byteIndex := x / 2

				if x%2 == 0 {
					paletteIndex = int(
						(row[byteIndex] >> 4) & 0x0f,
					)
				} else {
					paletteIndex = int(
						row[byteIndex] & 0x0f,
					)
				}

			case 8:
				paletteIndex = int(row[x])

			default:
				return nil, fmt.Errorf(
					"unsupported HII palette depth: %d",
					bitsPerPixel,
				)
			}

			if paletteIndex >= len(palette) {
				return nil, fmt.Errorf(
					"HII palette index %d is out of range",
					paletteIndex,
				)
			}

			output.SetNRGBA(
				x,
				y,
				palette[paletteIndex],
			)
		}
	}

	return output, nil
}

func readHIIPalette(
	data []byte,
	location hiiImageLocation,
) ([]color.NRGBA, error) {
	if location.palette == 0 {
		return nil, fmt.Errorf(
			"HII image refers to palette zero",
		)
	}

	if location.paletteInfoOffset == 0 {
		return nil, fmt.Errorf(
			"HII image package does not contain palette information",
		)
	}

	packageStart := location.packageOffset
	packageEnd := packageStart + location.packageLength
	paletteOffset := packageStart + location.paletteInfoOffset

	if packageStart < 0 ||
		packageEnd > len(data) ||
		paletteOffset < packageStart ||
		paletteOffset+2 > packageEnd {
		return nil, fmt.Errorf(
			"HII palette information is outside the image package",
		)
	}

	paletteCount := int(binary.LittleEndian.Uint16(
		data[paletteOffset : paletteOffset+2],
	))

	if paletteCount == 0 {
		return nil, fmt.Errorf(
			"HII image package contains no palettes",
		)
	}

	if int(location.palette) > paletteCount {
		return nil, fmt.Errorf(
			"HII palette index %d exceeds palette count %d",
			location.palette,
			paletteCount,
		)
	}

	offset := paletteOffset + 2

	for index := 1; index <= paletteCount; index++ {
		if offset+2 > packageEnd {
			return nil, fmt.Errorf(
				"HII palette header is incomplete",
			)
		}

		paletteSize := int(binary.LittleEndian.Uint16(
			data[offset : offset+2],
		))

		offset += 2

		if paletteSize < 0 ||
			offset+paletteSize > packageEnd {
			return nil, fmt.Errorf(
				"HII palette exceeds the image package",
			)
		}

		if paletteSize%3 != 0 {
			return nil, fmt.Errorf(
				"HII palette size %d is not divisible by three",
				paletteSize,
			)
		}

		if index == int(location.palette) {
			colorCount := paletteSize / 3

			palette := make(
				[]color.NRGBA,
				colorCount,
			)

			for colorIndex := 0; colorIndex < colorCount; colorIndex++ {
				colorOffset := offset + colorIndex*3

				palette[colorIndex] = color.NRGBA{
					B: data[colorOffset],
					G: data[colorOffset+1],
					R: data[colorOffset+2],
					A: 0xff,
				}
			}

			return palette, nil
		}

		offset += paletteSize
	}

	return nil, fmt.Errorf(
		"HII palette %d was not found",
		location.palette,
	)
}

func replaceHIIImage(
	data []byte,
	location hiiImageLocation,
	source image.Image,
) ([]byte, error) {
	newBlock, newPalette, err := encodeHIIImage(source)
	if err != nil {
		return nil, err
	}

	resource := location.resource

	resourceStart := resource.dataOffset
	resourceEnd := resourceStart + resource.dataSize

	if resourceStart < 0 ||
		resourceEnd > len(data) ||
		resourceStart >= resourceEnd {
		return nil, fmt.Errorf(
			"HII resource is outside the PE image",
		)
	}

	listLength := location.packageListLength
	listEnd := resourceStart + listLength

	if listLength < hiiPackageListHeaderSize ||
		listEnd > resourceEnd {
		return nil, fmt.Errorf(
			"invalid HII package list range",
		)
	}

	packageStart := location.packageOffset
	packageEnd := packageStart + location.packageLength

	if packageStart < resourceStart ||
		packageEnd > listEnd ||
		packageStart >= packageEnd {
		return nil, fmt.Errorf(
			"invalid HII image package range",
		)
	}

	blockStart := location.blockOffset
	blockEnd := blockStart + location.blockLength

	if blockStart < packageStart ||
		blockEnd > packageEnd ||
		blockStart >= blockEnd {
		return nil, fmt.Errorf(
			"invalid HII image block range",
		)
	}

	oldPackage := data[packageStart:packageEnd]

	blockRelativeStart := blockStart - packageStart
	blockRelativeEnd := blockEnd - packageStart
	blockDelta := len(newBlock) - location.blockLength

	imageInfoOffset := location.imageInfoOffset
	paletteInfoOffset := location.paletteInfoOffset

	var (
		paletteCount int
		paletteEnd   int
	)

	if newPalette != nil {
		if paletteInfoOffset == 0 {
			newBlock[1] = 1
		} else {
			paletteCount, paletteEnd, err = inspectHIIPaletteInfo(
				oldPackage,
				paletteInfoOffset,
			)
			if err != nil {
				return nil, err
			}

			if paletteCount >= maxUint8Value {
				return nil, fmt.Errorf(
					"HII image package already contains the maximum number of palettes",
				)
			}

			newBlock[1] = byte(paletteCount + 1)
		}
	}

	newPackage := make(
		[]byte,
		0,
		len(oldPackage)+blockDelta,
	)

	newPackage = append(
		newPackage,
		oldPackage[:blockRelativeStart]...,
	)

	newPackage = append(
		newPackage,
		newBlock...,
	)

	newPackage = append(
		newPackage,
		oldPackage[blockRelativeEnd:]...,
	)

	if paletteInfoOffset != 0 {
		switch {
		case paletteInfoOffset >= blockRelativeEnd:
			paletteInfoOffset += blockDelta

		case paletteInfoOffset > blockRelativeStart:
			return nil, fmt.Errorf(
				"HII palette information overlaps the image block",
			)
		}
	}

	if newPalette != nil {
		paletteEntry, err := encodeHIIPaletteEntry(newPalette)
		if err != nil {
			return nil, err
		}

		if paletteInfoOffset == 0 {
			paletteInfoOffset = len(newPackage)

			paletteInfo := make(
				[]byte,
				2,
				2+len(paletteEntry),
			)

			binary.LittleEndian.PutUint16(
				paletteInfo,
				1,
			)

			paletteInfo = append(
				paletteInfo,
				paletteEntry...,
			)

			newPackage = append(
				newPackage,
				paletteInfo...,
			)
		} else {
			paletteInsertOffset := paletteEnd

			switch {
			case paletteInsertOffset >= blockRelativeEnd:
				paletteInsertOffset += blockDelta

			case paletteInsertOffset > blockRelativeStart:
				return nil, fmt.Errorf(
					"HII palette information overlaps the image block",
				)
			}

			if paletteInsertOffset < paletteInfoOffset+2 ||
				paletteInsertOffset > len(newPackage) {
				return nil, fmt.Errorf(
					"invalid HII palette insertion offset",
				)
			}

			if paletteInfoOffset < hiiImagePackageHeaderSize ||
				paletteInfoOffset+2 > len(newPackage) {
				return nil, fmt.Errorf(
					"updated HII palette information offset is invalid",
				)
			}

			binary.LittleEndian.PutUint16(
				newPackage[paletteInfoOffset:paletteInfoOffset+2],
				uint16(paletteCount+1),
			)

			updatedPackage := make(
				[]byte,
				0,
				len(newPackage)+len(paletteEntry),
			)

			updatedPackage = append(
				updatedPackage,
				newPackage[:paletteInsertOffset]...,
			)

			updatedPackage = append(
				updatedPackage,
				paletteEntry...,
			)

			updatedPackage = append(
				updatedPackage,
				newPackage[paletteInsertOffset:]...,
			)

			newPackage = updatedPackage

			if paletteInsertOffset <= imageInfoOffset {
				imageInfoOffset += len(paletteEntry)
			}
		}
	}

	newPackageLength := len(newPackage)

	if newPackageLength < hiiImagePackageHeaderSize ||
		newPackageLength > maxUint24Value {
		return nil, fmt.Errorf(
			"updated HII image package is too large: %d bytes",
			newPackageLength,
		)
	}

	if imageInfoOffset < hiiImagePackageHeaderSize ||
		imageInfoOffset >= newPackageLength {
		return nil, fmt.Errorf(
			"updated HII image information offset is invalid",
		)
	}

	if paletteInfoOffset < 0 ||
		paletteInfoOffset >= newPackageLength {
		if paletteInfoOffset != 0 {
			return nil, fmt.Errorf(
				"updated HII palette information offset is invalid",
			)
		}
	}

	packageHeader := binary.LittleEndian.Uint32(
		newPackage[:hiiPackageHeaderSize],
	)

	packageHeader =
		(packageHeader & 0xff000000) |
			uint32(newPackageLength)

	binary.LittleEndian.PutUint32(
		newPackage[:hiiPackageHeaderSize],
		packageHeader,
	)

	binary.LittleEndian.PutUint32(
		newPackage[4:8],
		uint32(imageInfoOffset),
	)

	binary.LittleEndian.PutUint32(
		newPackage[8:12],
		uint32(paletteInfoOffset),
	)

	packageRelativeStart := packageStart - resourceStart
	packageRelativeEnd := packageEnd - resourceStart
	packageDelta := newPackageLength - location.packageLength
	newListLength := listLength + packageDelta

	if newListLength < hiiPackageListHeaderSize ||
		uint64(newListLength) > maxUint32Value {
		return nil, fmt.Errorf(
			"updated HII package list is too large: %d bytes",
			newListLength,
		)
	}

	oldList := data[resourceStart:listEnd]

	newList := make(
		[]byte,
		0,
		newListLength,
	)

	newList = append(
		newList,
		oldList[:packageRelativeStart]...,
	)

	newList = append(
		newList,
		newPackage...,
	)

	newList = append(
		newList,
		oldList[packageRelativeEnd:]...,
	)

	binary.LittleEndian.PutUint32(
		newList[16:20],
		uint32(newListLength),
	)

	oldResource := data[resourceStart:resourceEnd]

	newResource := make(
		[]byte,
		0,
		resource.dataSize+packageDelta,
	)

	newResource = append(
		newResource,
		newList...,
	)

	newResource = append(
		newResource,
		oldResource[listLength:]...,
	)

	return replacePEResourceData(
		data,
		resource,
		newResource,
	)
}

func inspectHIIPaletteInfo(
	packageData []byte,
	paletteInfoOffset int,
) (int, int, error) {
	if paletteInfoOffset < hiiImagePackageHeaderSize ||
		paletteInfoOffset+2 > len(packageData) {
		return 0, 0, fmt.Errorf(
			"invalid HII palette information offset: %d",
			paletteInfoOffset,
		)
	}

	paletteCount := int(binary.LittleEndian.Uint16(
		packageData[paletteInfoOffset : paletteInfoOffset+2],
	))

	if paletteCount == 0 {
		return 0, 0, fmt.Errorf(
			"HII image package contains no palettes",
		)
	}

	offset := paletteInfoOffset + 2

	for index := 0; index < paletteCount; index++ {
		if offset+2 > len(packageData) {
			return 0, 0, fmt.Errorf(
				"HII palette header is incomplete",
			)
		}

		paletteSize := int(binary.LittleEndian.Uint16(
			packageData[offset : offset+2],
		))

		offset += 2

		if paletteSize%3 != 0 {
			return 0, 0, fmt.Errorf(
				"HII palette size %d is not divisible by three",
				paletteSize,
			)
		}

		if offset+paletteSize > len(packageData) {
			return 0, 0, fmt.Errorf(
				"HII palette exceeds the image package",
			)
		}

		offset += paletteSize
	}

	return paletteCount, offset, nil
}

func encodeHIIImage(
	source image.Image,
) ([]byte, []byte, error) {
	block, palette, optimized, err := encodeHII8BitImage(
		source,
	)
	if err != nil {
		return nil, nil, err
	}

	if optimized {
		return block, palette, nil
	}

	block, err = encodeHII24BitImage(source)
	if err != nil {
		return nil, nil, err
	}

	return block, nil, nil
}

func encodeHII8BitImage(
	source image.Image,
) ([]byte, []byte, bool, error) {
	bounds, width, height, err := hiiImageDimensions(source)
	if err != nil {
		return nil, nil, false, err
	}

	pixelLength, err := checkedImageDataLength(
		width,
		height,
		8,
	)
	if err != nil {
		return nil, nil, false, err
	}

	block := make([]byte, 6+pixelLength)

	block[0] = hiiImage8Bit
	block[1] = 1
	binary.LittleEndian.PutUint16(
		block[2:4],
		uint16(width),
	)

	binary.LittleEndian.PutUint16(
		block[4:6],
		uint16(height),
	)

	paletteIndexes := make(
		map[uint32]byte,
		maxUint8Value+1,
	)

	palette := make(
		[]byte,
		0,
		(maxUint8Value+1)*3,
	)

	offset := 6

	for y := 0; y < height; y++ {
		sourceY := bounds.Min.Y + y

		for x := 0; x < width; x++ {
			sourceX := bounds.Min.X + x

			red, green, blue, _ := source.At(
				sourceX,
				sourceY,
			).RGBA()

			red8 := byte(red >> 8)
			green8 := byte(green >> 8)
			blue8 := byte(blue >> 8)

			key :=
				uint32(red8)<<16 |
					uint32(green8)<<8 |
					uint32(blue8)

			paletteIndex, found := paletteIndexes[key]
			if !found {
				colorCount := len(palette) / 3

				if colorCount > maxUint8Value {
					return nil, nil, false, nil
				}

				paletteIndex = byte(colorCount)
				paletteIndexes[key] = paletteIndex

				palette = append(
					palette,
					blue8,
					green8,
					red8,
				)
			}

			block[offset] = paletteIndex
			offset++
		}
	}

	return block, palette, true, nil
}

func encodeHIIPaletteEntry(
	palette []byte,
) ([]byte, error) {
	if len(palette) == 0 {
		return nil, fmt.Errorf(
			"HII palette is empty",
		)
	}

	if len(palette)%3 != 0 {
		return nil, fmt.Errorf(
			"HII palette size %d is not divisible by three",
			len(palette),
		)
	}

	if len(palette)/3 > maxUint8Value+1 {
		return nil, fmt.Errorf(
			"HII palette contains too many colors: %d",
			len(palette)/3,
		)
	}

	if len(palette) > maxUint16Value {
		return nil, fmt.Errorf(
			"HII palette is too large: %d bytes",
			len(palette),
		)
	}

	entry := make(
		[]byte,
		2,
		2+len(palette),
	)

	binary.LittleEndian.PutUint16(
		entry,
		uint16(len(palette)),
	)

	entry = append(
		entry,
		palette...,
	)

	return entry, nil
}

func hiiImageDimensions(
	source image.Image,
) (image.Rectangle, int, int, error) {
	if source == nil {
		return image.Rectangle{}, 0, 0, fmt.Errorf(
			"image is nil",
		)
	}

	bounds := source.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	if width <= 0 || height <= 0 {
		return image.Rectangle{}, 0, 0, fmt.Errorf(
			"image dimensions must be greater than zero",
		)
	}

	if width > maxUint16Value ||
		height > maxUint16Value {
		return image.Rectangle{}, 0, 0, fmt.Errorf(
			"image dimensions exceed the HII limit: %dx%d",
			width,
			height,
		)
	}

	return bounds, width, height, nil
}

func encodeHII24BitImage(
	source image.Image,
) ([]byte, error) {
	bounds, width, height, err := hiiImageDimensions(source)
	if err != nil {
		return nil, err
	}

	pixelLength, err := checkedImageDataLength(
		width,
		height,
		24,
	)
	if err != nil {
		return nil, err
	}

	block := make([]byte, 5+pixelLength)

	block[0] = hiiImage24Bit

	binary.LittleEndian.PutUint16(
		block[1:3],
		uint16(width),
	)

	binary.LittleEndian.PutUint16(
		block[3:5],
		uint16(height),
	)

	offset := 5

	for y := 0; y < height; y++ {
		sourceY := bounds.Min.Y + y

		for x := 0; x < width; x++ {
			sourceX := bounds.Min.X + x

			red, green, blue, _ := source.At(
				sourceX,
				sourceY,
			).RGBA()

			block[offset] = byte(blue >> 8)
			block[offset+1] = byte(green >> 8)
			block[offset+2] = byte(red >> 8)

			offset += 3
		}
	}

	return block, nil
}

func findHIIResource(
	data []byte,
) (peResourceInfo, error) {
	if len(data) < 24 {
		return peResourceInfo{}, fmt.Errorf(
			"PE image header is incomplete",
		)
	}

	peOffset := 0

	if len(data) >= 64 &&
		binary.LittleEndian.Uint16(data[0:2]) ==
			peDOSSignature {
		peOffset = int(binary.LittleEndian.Uint32(
			data[0x3c:0x40],
		))
	}

	if peOffset < 0 ||
		len(data)-peOffset < 24 {
		return peResourceInfo{}, fmt.Errorf(
			"PE header offset is outside the image",
		)
	}

	if binary.LittleEndian.Uint32(
		data[peOffset:peOffset+4],
	) != peSignature {
		return peResourceInfo{}, fmt.Errorf(
			"missing PE signature",
		)
	}

	fileHeaderOffset := peOffset + 4

	numberOfSections := int(binary.LittleEndian.Uint16(
		data[fileHeaderOffset+2 : fileHeaderOffset+4],
	))

	optionalHeaderSize := int(binary.LittleEndian.Uint16(
		data[fileHeaderOffset+16 : fileHeaderOffset+18],
	))

	optionalOffset :=
		fileHeaderOffset + peFileHeaderSize

	optionalEnd :=
		optionalOffset + optionalHeaderSize

	if optionalHeaderSize <= 0 ||
		optionalEnd > len(data) {
		return peResourceInfo{}, fmt.Errorf(
			"PE optional header is incomplete",
		)
	}

	magic := binary.LittleEndian.Uint16(
		data[optionalOffset : optionalOffset+2],
	)

	var (
		numberOfDirectoriesOffset int
		dataDirectoryOffset       int
	)

	switch magic {
	case peOptionalHeader32Magic:
		numberOfDirectoriesOffset =
			optionalOffset + 92

		dataDirectoryOffset =
			optionalOffset + 96

	case peOptionalHeader64Magic:
		numberOfDirectoriesOffset =
			optionalOffset + 108

		dataDirectoryOffset =
			optionalOffset + 112

	default:
		return peResourceInfo{}, fmt.Errorf(
			"unsupported PE optional header magic: %#04x",
			magic,
		)
	}

	if numberOfDirectoriesOffset+4 > optionalEnd {
		return peResourceInfo{}, fmt.Errorf(
			"PE data directory count is missing",
		)
	}

	numberOfDirectories := binary.LittleEndian.Uint32(
		data[numberOfDirectoriesOffset : numberOfDirectoriesOffset+4],
	)

	if numberOfDirectories <= peResourceDirectoryIndex {
		return peResourceInfo{},
			errHIIResourceNotFound
	}

	resourceDirectoryEntryOffset :=
		dataDirectoryOffset +
			peResourceDirectoryIndex*8

	if resourceDirectoryEntryOffset+8 > optionalEnd {
		return peResourceInfo{}, fmt.Errorf(
			"PE resource directory entry is incomplete",
		)
	}

	resourceRVA := binary.LittleEndian.Uint32(
		data[resourceDirectoryEntryOffset : resourceDirectoryEntryOffset+4],
	)

	resourceDirectorySize := binary.LittleEndian.Uint32(
		data[resourceDirectoryEntryOffset+4 : resourceDirectoryEntryOffset+8],
	)

	if resourceRVA == 0 ||
		resourceDirectorySize == 0 {
		return peResourceInfo{},
			errHIIResourceNotFound
	}

	if optionalOffset+68 > optionalEnd {
		return peResourceInfo{}, fmt.Errorf(
			"PE optional header fields are incomplete",
		)
	}

	sectionAlignment := binary.LittleEndian.Uint32(
		data[optionalOffset+32 : optionalOffset+36],
	)

	fileAlignment := binary.LittleEndian.Uint32(
		data[optionalOffset+36 : optionalOffset+40],
	)

	if sectionAlignment == 0 ||
		fileAlignment == 0 {
		return peResourceInfo{}, fmt.Errorf(
			"PE alignment fields must not be zero",
		)
	}

	sections, err := parsePESections(
		data,
		optionalEnd,
		numberOfSections,
	)
	if err != nil {
		return peResourceInfo{}, err
	}

	resourceBase, _, err := mapPERVAToOffset(
		resourceRVA,
		sections,
		len(data),
	)
	if err != nil {
		return peResourceInfo{}, fmt.Errorf(
			"map PE resource directory: %w",
			err,
		)
	}

	resourceLimit64 :=
		uint64(resourceBase) +
			uint64(resourceDirectorySize)

	if resourceLimit64 > uint64(len(data)) {
		return peResourceInfo{}, fmt.Errorf(
			"PE resource directory exceeds the image",
		)
	}

	resourceLimit := int(resourceLimit64)

	hiiEntryOffset, err := findHIIResourceEntry(
		data,
		resourceBase,
		resourceLimit,
	)
	if errors.Is(err, errHIIResourceNotFound) {
		return peResourceInfo{},
			errHIIResourceNotFound
	}

	if err != nil {
		return peResourceInfo{}, err
	}

	if hiiEntryOffset+peResourceDataEntrySize >
		resourceLimit {
		return peResourceInfo{}, fmt.Errorf(
			"PE HII resource data entry is incomplete",
		)
	}

	dataRVA := binary.LittleEndian.Uint32(
		data[hiiEntryOffset : hiiEntryOffset+4],
	)

	dataSize := int(binary.LittleEndian.Uint32(
		data[hiiEntryOffset+4 : hiiEntryOffset+8],
	))

	if dataSize <= 0 {
		return peResourceInfo{}, fmt.Errorf(
			"PE HII resource is empty",
		)
	}

	dataOffset, dataSection, err := mapPERVAToOffset(
		dataRVA,
		sections,
		len(data),
	)
	if err != nil {
		return peResourceInfo{}, fmt.Errorf(
			"map PE HII resource data: %w",
			err,
		)
	}

	dataEnd64 :=
		uint64(dataOffset) +
			uint64(dataSize)

	if dataEnd64 > uint64(len(data)) {
		return peResourceInfo{}, fmt.Errorf(
			"PE HII resource data exceeds the image",
		)
	}

	return peResourceInfo{
		sizeOfInitializedDataOffset: optionalOffset + 8,

		sizeOfImageOffset: optionalOffset + 56,

		checksumOffset: optionalOffset + 64,

		directorySizeOffset: resourceDirectoryEntryOffset + 4,

		fileAlignment: fileAlignment,

		sectionAlignment: sectionAlignment,

		directorySize: resourceDirectorySize,

		section: dataSection,

		sections: sections,

		dataEntryOffset: hiiEntryOffset,

		dataOffset: dataOffset,

		dataSize: dataSize,
	}, nil
}

func parsePESections(
	data []byte,
	offset int,
	count int,
) ([]peSectionInfo, error) {
	if count <= 0 {
		return nil, fmt.Errorf(
			"PE image has no sections",
		)
	}

	required64 :=
		uint64(offset) +
			uint64(count)*peSectionHeaderSize

	if offset < 0 ||
		required64 > uint64(len(data)) {
		return nil, fmt.Errorf(
			"PE section headers are incomplete",
		)
	}

	sections := make(
		[]peSectionInfo,
		0,
		count,
	)

	for index := 0; index < count; index++ {
		headerOffset :=
			offset + index*peSectionHeaderSize

		section := peSectionInfo{
			headerOffset: headerOffset,

			virtualSize: binary.LittleEndian.Uint32(
				data[headerOffset+8 : headerOffset+12],
			),

			virtualAddress: binary.LittleEndian.Uint32(
				data[headerOffset+12 : headerOffset+16],
			),

			rawSize: binary.LittleEndian.Uint32(
				data[headerOffset+16 : headerOffset+20],
			),

			rawOffset: binary.LittleEndian.Uint32(
				data[headerOffset+20 : headerOffset+24],
			),
		}

		if section.rawSize != 0 {
			rawEnd :=
				uint64(section.rawOffset) +
					uint64(section.rawSize)

			if rawEnd > uint64(len(data)) {
				return nil, fmt.Errorf(
					"PE section %d exceeds the image",
					index,
				)
			}
		}

		sections = append(
			sections,
			section,
		)
	}

	return sections, nil
}

func mapPERVAToOffset(
	rva uint32,
	sections []peSectionInfo,
	fileSize int,
) (int, peSectionInfo, error) {
	for _, section := range sections {
		span := section.virtualSize

		if section.rawSize > span {
			span = section.rawSize
		}

		if span == 0 ||
			rva < section.virtualAddress {
			continue
		}

		relative := uint64(
			rva - section.virtualAddress,
		)

		if relative >= uint64(span) {
			continue
		}

		if relative >= uint64(section.rawSize) {
			return 0, peSectionInfo{}, fmt.Errorf(
				"RVA %#x is not backed by file data",
				rva,
			)
		}

		offset64 :=
			uint64(section.rawOffset) +
				relative

		if offset64 >= uint64(fileSize) {
			return 0, peSectionInfo{}, fmt.Errorf(
				"RVA %#x maps outside the file",
				rva,
			)
		}

		return int(offset64), section, nil
	}

	return 0, peSectionInfo{}, fmt.Errorf(
		"RVA %#x does not belong to a PE section",
		rva,
	)
}

func findHIIResourceEntry(
	data []byte,
	resourceBase int,
	resourceLimit int,
) (int, error) {
	if resourceBase < 0 ||
		resourceLimit > len(data) ||
		resourceLimit-resourceBase <
			peResourceDirectoryHeaderSize {
		return 0, fmt.Errorf(
			"PE resource directory header is incomplete",
		)
	}

	entryOffset, err := findNamedPEResourceEntry(
		data,
		resourceBase,
		resourceLimit,
		resourceBase,
		"HII",
	)
	if err != nil {
		return 0, err
	}

	value := binary.LittleEndian.Uint32(
		data[entryOffset+4 : entryOffset+8],
	)

	for depth := 0; depth < 8; depth++ {
		if value&0x80000000 == 0 {
			dataEntryOffset :=
				resourceBase +
					int(value&0x7fffffff)

			if dataEntryOffset < resourceBase ||
				dataEntryOffset+
					peResourceDataEntrySize >
					resourceLimit {
				return 0, fmt.Errorf(
					"PE HII resource data entry is outside the resource directory",
				)
			}

			return dataEntryOffset, nil
		}

		directoryOffset :=
			int(value & 0x7fffffff)

		nextEntry, err :=
			firstPEResourceDirectoryEntry(
				data,
				resourceBase,
				resourceLimit,
				directoryOffset,
			)
		if err != nil {
			return 0, err
		}

		value = binary.LittleEndian.Uint32(
			data[nextEntry+4 : nextEntry+8],
		)
	}

	return 0, fmt.Errorf(
		"PE HII resource tree is too deeply nested",
	)
}

func findNamedPEResourceEntry(
	data []byte,
	resourceBase int,
	resourceLimit int,
	directory int,
	expected string,
) (int, error) {
	if directory < resourceBase ||
		directory+peResourceDirectoryHeaderSize >
			resourceLimit {
		return 0, fmt.Errorf(
			"PE resource directory is outside the resource section",
		)
	}

	namedEntries := int(binary.LittleEndian.Uint16(
		data[directory+12 : directory+14],
	))

	idEntries := int(binary.LittleEndian.Uint16(
		data[directory+14 : directory+16],
	))

	totalEntries := namedEntries + idEntries

	entriesEnd64 :=
		uint64(directory) +
			peResourceDirectoryHeaderSize +
			uint64(totalEntries)*
				peResourceDirectoryEntrySize

	if entriesEnd64 > uint64(resourceLimit) {
		return 0, fmt.Errorf(
			"PE resource directory entries are incomplete",
		)
	}

	for index := 0; index < namedEntries; index++ {
		entryOffset :=
			directory +
				peResourceDirectoryHeaderSize +
				index*peResourceDirectoryEntrySize

		nameValue := binary.LittleEndian.Uint32(
			data[entryOffset : entryOffset+4],
		)

		if nameValue&0x80000000 == 0 {
			continue
		}

		nameOffset :=
			int(nameValue & 0x7fffffff)

		name, err := readPEResourceName(
			data,
			resourceBase,
			resourceLimit,
			nameOffset,
		)
		if err != nil {
			return 0, err
		}

		if name == expected {
			return entryOffset, nil
		}
	}

	return 0, errHIIResourceNotFound
}

func firstPEResourceDirectoryEntry(
	data []byte,
	resourceBase int,
	resourceLimit int,
	directoryOffset int,
) (int, error) {
	directory :=
		resourceBase + directoryOffset

	if directory < resourceBase ||
		directory+peResourceDirectoryHeaderSize >
			resourceLimit {
		return 0, fmt.Errorf(
			"PE resource subdirectory is outside the resource directory",
		)
	}

	namedEntries := int(binary.LittleEndian.Uint16(
		data[directory+12 : directory+14],
	))

	idEntries := int(binary.LittleEndian.Uint16(
		data[directory+14 : directory+16],
	))

	if namedEntries+idEntries == 0 {
		return 0, fmt.Errorf(
			"PE resource subdirectory is empty",
		)
	}

	entry :=
		directory +
			peResourceDirectoryHeaderSize

	if entry+peResourceDirectoryEntrySize >
		resourceLimit {
		return 0, fmt.Errorf(
			"PE resource subdirectory entry is incomplete",
		)
	}

	return entry, nil
}

func readPEResourceName(
	data []byte,
	resourceBase int,
	resourceLimit int,
	nameOffset int,
) (string, error) {
	offset :=
		resourceBase + nameOffset

	if offset < resourceBase ||
		offset+2 > resourceLimit {
		return "", fmt.Errorf(
			"PE resource name is outside the resource directory",
		)
	}

	length := int(binary.LittleEndian.Uint16(
		data[offset : offset+2],
	))

	stringEnd64 :=
		uint64(offset+2) +
			uint64(length)*2

	if stringEnd64 > uint64(resourceLimit) {
		return "", fmt.Errorf(
			"PE resource name is incomplete",
		)
	}

	runes := make(
		[]rune,
		0,
		length,
	)

	for index := 0; index < length; index++ {
		characterOffset :=
			offset + 2 + index*2

		value := binary.LittleEndian.Uint16(
			data[characterOffset : characterOffset+2],
		)

		runes = append(
			runes,
			rune(value),
		)
	}

	return string(runes), nil
}

func replacePEResourceData(
	data []byte,
	resource peResourceInfo,
	newResource []byte,
) ([]byte, error) {
	if len(newResource) == 0 {
		return nil, fmt.Errorf(
			"updated HII resource is empty",
		)
	}

	if uint64(len(newResource)) > maxUint32Value {
		return nil, fmt.Errorf(
			"updated HII resource is too large",
		)
	}

	section := resource.section

	rawStart := int(section.rawOffset)
	oldRawSize := int(section.rawSize)
	oldRawEnd := rawStart + oldRawSize

	if rawStart < 0 ||
		oldRawSize <= 0 ||
		oldRawEnd > len(data) {
		return nil, fmt.Errorf(
			"PE resource section range is invalid",
		)
	}

	resourceRelativeStart :=
		resource.dataOffset - rawStart

	resourceRelativeEnd :=
		resourceRelativeStart +
			resource.dataSize

	if resourceRelativeStart < 0 ||
		resourceRelativeEnd > oldRawSize {
		return nil, fmt.Errorf(
			"HII resource is outside its PE section",
		)
	}

	virtualLimit := int(section.virtualSize)

	if virtualLimit <= 0 ||
		virtualLimit > oldRawSize {
		virtualLimit = oldRawSize
	}

	if resourceRelativeEnd < virtualLimit &&
		containsNonZero(
			data[rawStart+resourceRelativeEnd:rawStart+virtualLimit],
		) {
		return nil, fmt.Errorf(
			"HII resource is not the final object in its PE section",
		)
	}

	requiredVirtualSize :=
		resourceRelativeStart +
			len(newResource)

	newVirtualSize := int(section.virtualSize)

	if newVirtualSize < requiredVirtualSize {
		newVirtualSize = requiredVirtualSize
	}

	if uint64(newVirtualSize) > maxUint32Value {
		return nil, fmt.Errorf(
			"updated PE resource section is too large",
		)
	}

	if err := checkPEVirtualGrowth(
		resource.sections,
		section,
		uint32(newVirtualSize),
	); err != nil {
		return nil, err
	}

	newRawSize := oldRawSize

	if requiredVirtualSize > oldRawSize {
		alignedRawSize, err := alignValue(
			uint64(requiredVirtualSize),
			uint64(resource.fileAlignment),
		)
		if err != nil {
			return nil, err
		}

		if alignedRawSize > maxUint32Value ||
			alignedRawSize > uint64(maxIntValue()) {
			return nil, fmt.Errorf(
				"updated PE resource section is too large",
			)
		}

		newRawSize = int(alignedRawSize)
	}

	rawDelta :=
		newRawSize - oldRawSize

	newLength64 :=
		uint64(len(data)) +
			uint64(rawDelta)

	if newLength64 > uint64(maxIntValue()) {
		return nil, fmt.Errorf(
			"updated PE image exceeds the platform size limit",
		)
	}

	output := make(
		[]byte,
		int(newLength64),
	)

	copy(
		output[:rawStart],
		data[:rawStart],
	)

	copy(
		output[rawStart:rawStart+resourceRelativeStart],
		data[rawStart:rawStart+resourceRelativeStart],
	)

	copy(
		output[rawStart+resourceRelativeStart:rawStart+resourceRelativeStart+len(newResource)],
		newResource,
	)

	copy(
		output[rawStart+newRawSize:],
		data[oldRawEnd:],
	)

	binary.LittleEndian.PutUint32(
		output[section.headerOffset+8:section.headerOffset+12],
		uint32(newVirtualSize),
	)

	binary.LittleEndian.PutUint32(
		output[section.headerOffset+16:section.headerOffset+20],
		uint32(newRawSize),
	)

	if rawDelta != 0 {
		for _, other := range resource.sections {
			if other.rawSize == 0 ||
				other.rawOffset <= section.rawOffset {
				continue
			}

			newRawOffset64 :=
				uint64(other.rawOffset) +
					uint64(rawDelta)

			if newRawOffset64 > maxUint32Value {
				return nil, fmt.Errorf(
					"updated PE section offset is too large",
				)
			}

			binary.LittleEndian.PutUint32(
				output[other.headerOffset+20:other.headerOffset+24],
				uint32(newRawOffset64),
			)
		}
	}

	binary.LittleEndian.PutUint32(
		output[resource.dataEntryOffset+4:resource.dataEntryOffset+8],
		uint32(len(newResource)),
	)

	resourceDelta :=
		int64(len(newResource)) -
			int64(resource.dataSize)

	newDirectorySize :=
		int64(resource.directorySize) +
			resourceDelta

	if newDirectorySize <= 0 ||
		uint64(newDirectorySize) >
			maxUint32Value {
		return nil, fmt.Errorf(
			"updated PE resource directory size is invalid",
		)
	}

	binary.LittleEndian.PutUint32(
		output[resource.directorySizeOffset:resource.directorySizeOffset+4],
		uint32(newDirectorySize),
	)

	oldInitializedSize := int64(
		binary.LittleEndian.Uint32(
			output[resource.sizeOfInitializedDataOffset : resource.sizeOfInitializedDataOffset+4],
		),
	)

	newInitializedSize :=
		oldInitializedSize +
			int64(rawDelta)

	if newInitializedSize < 0 ||
		uint64(newInitializedSize) >
			maxUint32Value {
		return nil, fmt.Errorf(
			"updated PE initialized data size is invalid",
		)
	}

	binary.LittleEndian.PutUint32(
		output[resource.sizeOfInitializedDataOffset:resource.sizeOfInitializedDataOffset+4],
		uint32(newInitializedSize),
	)

	newSizeOfImage, err := calculatePESizeOfImage(
		resource.sections,
		section.headerOffset,
		uint32(newVirtualSize),
		resource.sectionAlignment,
	)
	if err != nil {
		return nil, err
	}

	binary.LittleEndian.PutUint32(
		output[resource.sizeOfImageOffset:resource.sizeOfImageOffset+4],
		newSizeOfImage,
	)

	binary.LittleEndian.PutUint32(
		output[resource.checksumOffset:resource.checksumOffset+4],
		0,
	)

	return output, nil
}

func checkPEVirtualGrowth(
	sections []peSectionInfo,
	updated peSectionInfo,
	newVirtualSize uint32,
) error {
	var nextAddress uint32

	for _, section := range sections {
		if section.virtualAddress <=
			updated.virtualAddress {
			continue
		}

		if nextAddress == 0 ||
			section.virtualAddress < nextAddress {
			nextAddress =
				section.virtualAddress
		}
	}

	if nextAddress == 0 {
		return nil
	}

	available :=
		uint64(nextAddress) -
			uint64(updated.virtualAddress)

	if uint64(newVirtualSize) > available {
		return fmt.Errorf(
			"updated HII resource does not fit before the next PE section",
		)
	}

	return nil
}

func calculatePESizeOfImage(
	sections []peSectionInfo,
	updatedHeaderOffset int,
	updatedVirtualSize uint32,
	sectionAlignment uint32,
) (uint32, error) {
	var maximum uint64

	for _, section := range sections {
		virtualSize := section.virtualSize

		if section.headerOffset ==
			updatedHeaderOffset {
			virtualSize =
				updatedVirtualSize
		}

		if virtualSize == 0 {
			virtualSize =
				section.rawSize
		}

		end :=
			uint64(section.virtualAddress) +
				uint64(virtualSize)

		aligned, err := alignValue(
			end,
			uint64(sectionAlignment),
		)
		if err != nil {
			return 0, err
		}

		if aligned > maximum {
			maximum = aligned
		}
	}

	if maximum == 0 ||
		maximum > maxUint32Value {
		return 0, fmt.Errorf(
			"updated PE image size is invalid",
		)
	}

	return uint32(maximum), nil
}

func checkedImageDataLength(
	width int,
	height int,
	bitsPerPixel uint64,
) (int, error) {
	if width <= 0 || height <= 0 {
		return 0, fmt.Errorf(
			"image dimensions must be greater than zero",
		)
	}

	rowBits :=
		uint64(width) *
			bitsPerPixel

	rowBytes :=
		(rowBits + 7) / 8

	size :=
		rowBytes *
			uint64(height)

	if size > uint64(maxIntValue()) {
		return 0, fmt.Errorf(
			"image pixel data is too large",
		)
	}

	return int(size), nil
}

func checkBlockRange(
	offset int,
	length int,
	limit int,
) error {
	if length <= 0 ||
		offset < 0 ||
		offset > limit ||
		length > limit-offset {
		return fmt.Errorf(
			"HII image block exceeds its image package",
		)
	}

	return nil
}

func alignValue(
	value uint64,
	alignment uint64,
) (uint64, error) {
	if alignment == 0 {
		return 0, fmt.Errorf(
			"alignment must not be zero",
		)
	}

	remainder :=
		value % alignment

	if remainder == 0 {
		return value, nil
	}

	increment :=
		alignment - remainder

	if value > ^uint64(0)-increment {
		return 0, fmt.Errorf(
			"aligned value overflows",
		)
	}

	return value + increment, nil
}

func containsNonZero(
	data []byte,
) bool {
	for _, value := range data {
		if value != 0 {
			return true
		}
	}

	return false
}

func maxIntValue() int {
	return int(^uint(0) >> 1)
}
