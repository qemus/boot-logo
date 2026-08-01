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
	peDOSSignature              = 0x5a4d
	peSignature                 = 0x00004550
	peOptionalHeader32Magic     = 0x010b
	peOptionalHeader64Magic     = 0x020b
	peFileHeaderSize            = 20
	peSectionHeaderSize         = 40
	peResourceDirectoryIndex    = 2
	peResourceDirectorySize     = 16
	peResourceDirectoryEntrySize = 8
	peResourceDataEntrySize     = 16

	hiiPackageListHeaderSize = 20
	hiiPackageHeaderSize     = 4
	hiiImagePackageHeaderSize = 12

	hiiPackageImages = 0x06
	hiiPackageEnd    = 0xdf

	hiiImageEnd          = 0x00
	hiiImage1Bit         = 0x10
	hiiImage1BitTrans    = 0x11
	hiiImage4Bit         = 0x12
	hiiImage4BitTrans    = 0x13
	hiiImage8Bit         = 0x14
	hiiImage8BitTrans    = 0x15
	hiiImage24Bit        = 0x16
	hiiImage24BitTrans   = 0x17
	hiiImageJPEG         = 0x18
	hiiImagePNG          = 0x19
	hiiImageDuplicate    = 0x20
	hiiImageSkip2        = 0x21
	hiiImageSkip1        = 0x22
	hiiImageExt1         = 0x30
	hiiImageExt2         = 0x31
	hiiImageExt4         = 0x32

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

	blockOffset int
	blockLength int
	blockType   byte

	width  int
	height int
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

	if resource.dataSize < hiiPackageListHeaderSize {
		return nil, fmt.Errorf(
			"HII package list header is incomplete",
		)
	}

	listStart := resource.dataOffset
	listLimit := listStart + resource.dataSize

	listLength := int(binary.LittleEndian.Uint32(
		data[listStart+16 : listStart+20],
	))

	if listLength < hiiPackageListHeaderSize {
		return nil, fmt.Errorf(
			"invalid HII package list length: %d",
			listLength,
		)
	}

	listEnd := listStart + listLength

	if listEnd > listLimit {
		return nil, fmt.Errorf(
			"HII package list length %d exceeds resource size %d",
			listLength,
			resource.dataSize,
		)
	}

	var images []hiiImageLocation

	for offset := listStart + hiiPackageListHeaderSize; offset < listEnd; {
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

			images = append(images, packageImages...)

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

	imageInfoOffset := int(binary.LittleEndian.Uint32(
		data[packageOffset+4 : packageOffset+8],
	))

	if imageInfoOffset < hiiImagePackageHeaderSize ||
		imageInfoOffset >= packageLength {
		return nil, fmt.Errorf(
			"invalid HII image information offset: %d",
			imageInfoOffset,
		)
	}

	var images []hiiImageLocation

	for offset := packageOffset + imageInfoOffset; offset < packageEnd; {
		blockType := data[offset]

		if blockType == hiiImageEnd {
			return images, nil
		}

		blockLength, width, height, concrete, err :=
			parseHIIImageBlock(
				data,
				offset,
				packageEnd,
			)
		if err != nil {
			return nil, err
		}

		if concrete {
			images = append(
				images,
				hiiImageLocation{
					resource: resource,

					packageListLength: listLength,

					packageOffset: packageOffset,
					packageLength: packageLength,

					blockOffset: offset,
					blockLength: blockLength,
					blockType:   blockType,

					width:  width,
					height: height,
				},
			)
		}

		offset += blockLength
	}

	return nil, fmt.Errorf(
		"HII image package does not contain an end block",
	)
}

func parseHIIImageBlock(
	data []byte,
	offset int,
	limit int,
) (int, int, int, bool, error) {
	if offset < 0 || offset >= limit || limit > len(data) {
		return 0, 0, 0, false, fmt.Errorf(
			"HII image block offset is outside the package",
		)
	}

	blockType := data[offset]

	switch blockType {
	case hiiImage1Bit, hiiImage1BitTrans:
		width, height, err := readHIIImageDimensions(
			data,
			offset,
			limit,
		)
		if err != nil {
			return 0, 0, 0, false, err
		}

		pixelLength, err := checkedImageDataLength(
			width,
			height,
			1,
		)
		if err != nil {
			return 0, 0, 0, false, err
		}

		blockLength := 5 + pixelLength

		if err := checkBlockRange(
			offset,
			blockLength,
			limit,
		); err != nil {
			return 0, 0, 0, false, err
		}

		return blockLength, width, height, true, nil

	case hiiImage4Bit, hiiImage4BitTrans:
		width, height, err := readHIIImageDimensions(
			data,
			offset,
			limit,
		)
		if err != nil {
			return 0, 0, 0, false, err
		}

		pixelLength, err := checkedImageDataLength(
			width,
			height,
			4,
		)
		if err != nil {
			return 0, 0, 0, false, err
		}

		blockLength := 5 + pixelLength

		if err := checkBlockRange(
			offset,
			blockLength,
			limit,
		); err != nil {
			return 0, 0, 0, false, err
		}

		return blockLength, width, height, true, nil

	case hiiImage8Bit, hiiImage8BitTrans:
		width, height, err := readHIIImageDimensions(
			data,
			offset,
			limit,
		)
		if err != nil {
			return 0, 0, 0, false, err
		}

		pixelLength, err := checkedImageDataLength(
			width,
			height,
			8,
		)
		if err != nil {
			return 0, 0, 0, false, err
		}

		blockLength := 5 + pixelLength

		if err := checkBlockRange(
			offset,
			blockLength,
			limit,
		); err != nil {
			return 0, 0, 0, false, err
		}

		return blockLength, width, height, true, nil

	case hiiImage24Bit, hiiImage24BitTrans:
		width, height, err := readHIIImageDimensions(
			data,
			offset,
			limit,
		)
		if err != nil {
			return 0, 0, 0, false, err
		}

		pixelLength, err := checkedImageDataLength(
			width,
			height,
			24,
		)
		if err != nil {
			return 0, 0, 0, false, err
		}

		blockLength := 5 + pixelLength

		if err := checkBlockRange(
			offset,
			blockLength,
			limit,
		); err != nil {
			return 0, 0, 0, false, err
		}

		return blockLength, width, height, true, nil

	case hiiImageJPEG, hiiImagePNG:
		if limit-offset < 5 {
			return 0, 0, 0, false, fmt.Errorf(
				"compressed HII image block is incomplete",
			)
		}

		payloadLength := uint64(binary.LittleEndian.Uint32(
			data[offset+1 : offset+5],
		))

		blockLength64 := uint64(5) + payloadLength

		if blockLength64 > uint64(^uint(0)>>1) {
			return 0, 0, 0, false, fmt.Errorf(
				"compressed HII image is too large",
			)
		}

		blockLength := int(blockLength64)

		if err := checkBlockRange(
			offset,
			blockLength,
			limit,
		); err != nil {
			return 0, 0, 0, false, err
		}

		return blockLength, 0, 0, true, nil

	case hiiImageDuplicate:
		const blockLength = 3

		if err := checkBlockRange(
			offset,
			blockLength,
			limit,
		); err != nil {
			return 0, 0, 0, false, err
		}

		return blockLength, 0, 0, false, nil

	case hiiImageSkip1:
		const blockLength = 2

		if err := checkBlockRange(
			offset,
			blockLength,
			limit,
		); err != nil {
			return 0, 0, 0, false, err
		}

		return blockLength, 0, 0, false, nil

	case hiiImageSkip2:
		const blockLength = 3

		if err := checkBlockRange(
			offset,
			blockLength,
			limit,
		); err != nil {
			return 0, 0, 0, false, err
		}

		return blockLength, 0, 0, false, nil

	case hiiImageExt1:
		if limit-offset < 3 {
			return 0, 0, 0, false, fmt.Errorf(
				"HII EXT1 image block is incomplete",
			)
		}

		blockLength := int(data[offset+2])

		if blockLength < 3 {
			return 0, 0, 0, false, fmt.Errorf(
				"invalid HII EXT1 block length: %d",
				blockLength,
			)
		}

		if err := checkBlockRange(
			offset,
			blockLength,
			limit,
		); err != nil {
			return 0, 0, 0, false, err
		}

		return blockLength, 0, 0, false, nil

	case hiiImageExt2:
		if limit-offset < 4 {
			return 0, 0, 0, false, fmt.Errorf(
				"HII EXT2 image block is incomplete",
			)
		}

		blockLength := int(binary.LittleEndian.Uint16(
			data[offset+2 : offset+4],
		))

		if blockLength < 4 {
			return 0, 0, 0, false, fmt.Errorf(
				"invalid HII EXT2 block length: %d",
				blockLength,
			)
		}

		if err := checkBlockRange(
			offset,
			blockLength,
			limit,
		); err != nil {
			return 0, 0, 0, false, err
		}

		return blockLength, 0, 0, false, nil

	case hiiImageExt4:
		if limit-offset < 6 {
			return 0, 0, 0, false, fmt.Errorf(
				"HII EXT4 image block is incomplete",
			)
		}

		blockLength64 := uint64(binary.LittleEndian.Uint32(
			data[offset+2 : offset+6],
		))

		if blockLength64 < 6 {
			return 0, 0, 0, false, fmt.Errorf(
				"invalid HII EXT4 block length: %d",
				blockLength64,
			)
		}

		if blockLength64 > uint64(^uint(0)>>1) {
			return 0, 0, 0, false, fmt.Errorf(
				"HII EXT4 image block is too large",
			)
		}

		blockLength := int(blockLength64)

		if err := checkBlockRange(
			offset,
			blockLength,
			limit,
		); err != nil {
			return 0, 0, 0, false, err
		}

		return blockLength, 0, 0, false, nil

	default:
		return 0, 0, 0, false, fmt.Errorf(
			"unsupported HII image block type: %#02x",
			blockType,
		)
	}
}

func decodeHIIImage(
	data []byte,
	location hiiImageLocation,
) (image.Image, error) {
	start := location.blockOffset
	end := start + location.blockLength

	if start < 0 || end > len(data) || start >= end {
		return nil, fmt.Errorf(
			"HII image block is outside the PE image",
		)
	}

	block := data[start:end]

	switch location.blockType {
	case hiiImage24Bit, hiiImage24BitTrans:
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

		pixelOffset := 5

		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				output.SetNRGBA(
					x,
					y,
					color.NRGBA{
						B: block[pixelOffset],
						G: block[pixelOffset+1],
						R: block[pixelOffset+2],
						A: 0xff,
					},
				)

				pixelOffset += 3
			}
		}

		return output, nil

	case hiiImageJPEG:
		if len(block) < 5 {
			return nil, fmt.Errorf(
				"JPEG HII image block is incomplete",
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

		return png.Decode(
			bytes.NewReader(block[5:]),
		)

	case hiiImage1Bit,
		hiiImage1BitTrans,
		hiiImage4Bit,
		hiiImage4BitTrans,
		hiiImage8Bit,
		hiiImage8BitTrans:
		return nil, fmt.Errorf(
			"palette-based HII image blocks are not supported yet",
		)

	default:
		return nil, fmt.Errorf(
			"unsupported HII image block type: %#02x",
			location.blockType,
		)
	}
}

func replaceHIIImage(
	data []byte,
	location hiiImageLocation,
	source image.Image,
) ([]byte, error) {
	newBlock, err := encodeHII24BitImage(source)
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

	blockStart := location.blockOffset
	blockEnd := blockStart + location.blockLength

	if blockStart < resourceStart ||
		blockEnd > listEnd ||
		blockStart >= blockEnd {
		return nil, fmt.Errorf(
			"invalid HII image block range",
		)
	}

	delta := len(newBlock) - location.blockLength

	newPackageLength := location.packageLength + delta
	if newPackageLength < hiiImagePackageHeaderSize ||
		newPackageLength > maxUint24Value {
		return nil, fmt.Errorf(
			"updated HII image package is too large: %d bytes",
			newPackageLength,
		)
	}

	newListLength := listLength + delta
	if newListLength < hiiPackageListHeaderSize ||
		uint64(newListLength) > maxUint32Value {
		return nil, fmt.Errorf(
			"updated HII package list is too large: %d bytes",
			newListLength,
		)
	}

	oldList := data[resourceStart:listEnd]

	blockRelativeStart := blockStart - resourceStart
	blockRelativeEnd := blockEnd - resourceStart

	newList := make(
		[]byte,
		0,
		newListLength,
	)

	newList = append(
		newList,
		oldList[:blockRelativeStart]...,
	)

	newList = append(
		newList,
		newBlock...,
	)

	newList = append(
		newList,
		oldList[blockRelativeEnd:]...,
	)

	packageRelativeOffset :=
		location.packageOffset - resourceStart

	if packageRelativeOffset < 0 ||
		packageRelativeOffset+hiiPackageHeaderSize > len(newList) {
		return nil, fmt.Errorf(
			"invalid HII image package offset",
		)
	}

	packageHeader := binary.LittleEndian.Uint32(
		newList[
			packageRelativeOffset :
				packageRelativeOffset+hiiPackageHeaderSize
		],
	)

	packageHeader =
		(packageHeader & 0xff000000) |
			uint32(newPackageLength)

	binary.LittleEndian.PutUint32(
		newList[
			packageRelativeOffset :
				packageRelativeOffset+hiiPackageHeaderSize
		],
		packageHeader,
	)

	binary.LittleEndian.PutUint32(
		newList[16:20],
		uint32(newListLength),
	)

	oldResource := data[resourceStart:resourceEnd]

	newResource := make(
		[]byte,
		0,
		resource.dataSize+delta,
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

func encodeHII24BitImage(
	source image.Image,
) ([]byte, error) {
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

	if width > maxUint16Value ||
		height > maxUint16Value {
		return nil, fmt.Errorf(
			"image dimensions exceed the HII limit: %dx%d",
			width,
			height,
		)
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

	if peOffset < 0 || len(data)-peOffset < 24 {
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

	optionalEnd := optionalOffset + optionalHeaderSize

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
		numberOfDirectoriesOffset = optionalOffset + 92
		dataDirectoryOffset = optionalOffset + 96

	case peOptionalHeader64Magic:
		numberOfDirectoriesOffset = optionalOffset + 108
		dataDirectoryOffset = optionalOffset + 112

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
		data[
			numberOfDirectoriesOffset :
				numberOfDirectoriesOffset+4
		],
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
		data[
			resourceDirectoryEntryOffset :
				resourceDirectoryEntryOffset+4
		],
	)

	resourceDirectorySizeValue :=
		binary.LittleEndian.Uint32(
			data[
				resourceDirectoryEntryOffset+4 :
					resourceDirectoryEntryOffset+8
			],
		)

	if resourceRVA == 0 ||
		resourceDirectorySizeValue == 0 {
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

	if sectionAlignment == 0 || fileAlignment == 0 {
		return peResourceInfo{}, fmt.Errorf(
			"PE alignment fields must not be zero",
		)
	}

	sectionHeadersOffset := optionalEnd

	sections, err := parsePESections(
		data,
		sectionHeadersOffset,
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
			uint64(resourceDirectorySizeValue)

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
		uint64(dataOffset) + uint64(dataSize)

	if dataEnd64 > uint64(len(data)) {
		return peResourceInfo{}, fmt.Errorf(
			"PE HII resource data exceeds the image",
		)
	}

	return peResourceInfo{
		sizeOfInitializedDataOffset: optionalOffset + 8,
		sizeOfImageOffset:           optionalOffset + 56,
		checksumOffset:              optionalOffset + 64,
		directorySizeOffset:
			resourceDirectoryEntryOffset + 4,

		fileAlignment:    fileAlignment,
		sectionAlignment: sectionAlignment,
		directorySize:    resourceDirectorySizeValue,

		section:  dataSection,
		sections: sections,

		dataEntryOffset: hiiEntryOffset,
		dataOffset:      dataOffset,
		dataSize:        dataSize,
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

	if offset < 0 || required64 > uint64(len(data)) {
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
				data[
					headerOffset+8 :
						headerOffset+12
				],
			),

			virtualAddress: binary.LittleEndian.Uint32(
				data[
					headerOffset+12 :
						headerOffset+16
				],
			),

			rawSize: binary.LittleEndian.Uint32(
				data[
					headerOffset+16 :
						headerOffset+20
				],
			),

			rawOffset: binary.LittleEndian.Uint32(
				data[
					headerOffset+20 :
						headerOffset+24
				],
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

		sections = append(sections, section)
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

		if span == 0 || rva < section.virtualAddress {
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
			uint64(section.rawOffset) + relative

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
			peResourceDirectorySize {
		return 0, fmt.Errorf(
			"PE resource directory header is incomplete",
		)
	}

	namedEntries := int(binary.LittleEndian.Uint16(
		data[
			resourceBase+12 :
				resourceBase+14
		],
	))

	idEntries := int(binary.LittleEndian.Uint16(
		data[
			resourceBase+14 :
				resourceBase+16
		],
	))

	totalEntries := namedEntries + idEntries

	entriesEnd64 :=
		uint64(resourceBase) +
			peResourceDirectorySize +
			uint64(totalEntries)*
				peResourceDirectoryEntrySize

	if entriesEnd64 > uint64(resourceLimit) {
		return 0, fmt.Errorf(
			"PE resource directory entries are incomplete",
		)
	}

	for index := 0; index < namedEntries; index++ {
		entryOffset :=
			resourceBase +
				peResourceDirectorySize +
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

		if name != "HII" {
			continue
		}

		value := binary.LittleEndian.Uint32(
			data[entryOffset+4 : entryOffset+8],
		)

		for level := 0;
			level < 2 &&
				value&0x80000000 != 0;
			level++ {
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

		if value&0x80000000 != 0 {
			return 0, fmt.Errorf(
				"PE HII resource tree is too deeply nested",
			)
		}

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
		directory+peResourceDirectorySize >
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
		directory + peResourceDirectorySize

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
	offset := resourceBase + nameOffset

	if offset < resourceBase || offset+2 > resourceLimit {
		return "", fmt.Errorf(
			"PE resource name is outside the resource directory",
		)
	}

	length := int(binary.LittleEndian.Uint16(
		data[offset : offset+2],
	))

	stringEnd64 :=
		uint64(offset+2) + uint64(length)*2

	if stringEnd64 > uint64(resourceLimit) {
		return "", fmt.Errorf(
			"PE resource name is incomplete",
		)
	}

	runes := make([]rune, 0, length)

	for index := 0; index < length; index++ {
		value := binary.LittleEndian.Uint16(
			data[
				offset+2+index*2 :
					offset+4+index*2
			],
		)

		runes = append(runes, rune(value))
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
		resourceRelativeStart + resource.dataSize

	if resourceRelativeStart < 0 ||
		resourceRelativeEnd > oldRawSize {
		return nil, fmt.Errorf(
			"HII resource is outside its PE section",
		)
	}

	virtualLimit := int(section.virtualSize)
	if virtualLimit <= 0 || virtualLimit > oldRawSize {
		virtualLimit = oldRawSize
	}

	if resourceRelativeEnd < virtualLimit &&
		containsNonZero(
			data[
				rawStart+resourceRelativeEnd :
					rawStart+virtualLimit
			],
		) {
		return nil, fmt.Errorf(
			"HII resource is not the final object in its PE section",
		)
	}

	for _, other := range resource.sections {
		if other.rawSize == 0 {
			continue
		}

		if other.rawOffset > section.rawOffset {
			return nil, fmt.Errorf(
				"PE resource section is not the final file section",
			)
		}
	}

	if oldRawEnd < len(data) &&
		containsNonZero(data[oldRawEnd:]) {
		return nil, fmt.Errorf(
			"PE image contains data after its final section",
		)
	}

	newVirtualSize :=
		resourceRelativeStart + len(newResource)

	if uint64(newVirtualSize) > maxUint32Value {
		return nil, fmt.Errorf(
			"updated PE resource section is too large",
		)
	}

	newRawSize64, err := alignValue(
		uint64(newVirtualSize),
		uint64(resource.fileAlignment),
	)
	if err != nil {
		return nil, err
	}

	if newRawSize64 > maxUint32Value ||
		newRawSize64 > uint64(^uint(0)>>1) {
		return nil, fmt.Errorf(
			"updated PE resource section is too large",
		)
	}

	newRawSize := int(newRawSize64)

	newLength64 :=
		uint64(len(data)-oldRawSize) +
			uint64(newRawSize)

	if newLength64 > uint64(^uint(0)>>1) {
		return nil, fmt.Errorf(
			"updated PE image exceeds the platform size limit",
		)
	}

	output := make([]byte, int(newLength64))

	copy(output[:rawStart], data[:rawStart])

	copy(
		output[
			rawStart :
				rawStart+resourceRelativeStart
		],
		data[
			rawStart :
				rawStart+resourceRelativeStart
		],
	)

	copy(
		output[
			rawStart+resourceRelativeStart :
				rawStart+resourceRelativeStart+
					len(newResource)
		],
		newResource,
	)

	copy(
		output[rawStart+newRawSize:],
		data[oldRawEnd:],
	)

	binary.LittleEndian.PutUint32(
		output[
			section.headerOffset+8 :
				section.headerOffset+12
		],
		uint32(newVirtualSize),
	)

	binary.LittleEndian.PutUint32(
		output[
			section.headerOffset+16 :
				section.headerOffset+20
		],
		uint32(newRawSize),
	)

	binary.LittleEndian.PutUint32(
		output[
			resource.dataEntryOffset+4 :
				resource.dataEntryOffset+8
		],
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
		output[
			resource.directorySizeOffset :
				resource.directorySizeOffset+4
		],
		uint32(newDirectorySize),
	)

	rawDelta :=
		int64(newRawSize) -
			int64(oldRawSize)

	oldInitializedSize :=
		int64(binary.LittleEndian.Uint32(
			output[
				resource.sizeOfInitializedDataOffset :
					resource.sizeOfInitializedDataOffset+4
			],
		))

	newInitializedSize :=
		oldInitializedSize + rawDelta

	if newInitializedSize < 0 ||
		uint64(newInitializedSize) >
			maxUint32Value {
		return nil, fmt.Errorf(
			"updated PE initialized data size is invalid",
		)
	}

	binary.LittleEndian.PutUint32(
		output[
			resource.sizeOfInitializedDataOffset :
				resource.sizeOfInitializedDataOffset+4
		],
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
		output[
			resource.sizeOfImageOffset :
				resource.sizeOfImageOffset+4
		],
		newSizeOfImage,
	)

	binary.LittleEndian.PutUint32(
		output[
			resource.checksumOffset :
				resource.checksumOffset+4
		],
		0,
	)

	return output, nil
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
			virtualSize = updatedVirtualSize
		}

		if virtualSize == 0 {
			virtualSize = section.rawSize
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

	if maximum == 0 || maximum > maxUint32Value {
		return 0, fmt.Errorf(
			"updated PE image size is invalid",
		)
	}

	return uint32(maximum), nil
}

func readHIIImageDimensions(
	data []byte,
	offset int,
	limit int,
) (int, int, error) {
	if offset < 0 ||
		limit-offset < 5 ||
		limit > len(data) {
		return 0, 0, fmt.Errorf(
			"HII image dimensions are incomplete",
		)
	}

	width := int(binary.LittleEndian.Uint16(
		data[offset+1 : offset+3],
	))

	height := int(binary.LittleEndian.Uint16(
		data[offset+3 : offset+5],
	))

	if width <= 0 || height <= 0 {
		return 0, 0, fmt.Errorf(
			"HII image dimensions must be greater than zero",
		)
	}

	return width, height, nil
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
		uint64(width) * bitsPerPixel

	rowBytes :=
		(rowBits + 7) / 8

	size :=
		rowBytes * uint64(height)

	if size > uint64(^uint(0)>>1) {
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

	remainder := value % alignment
	if remainder == 0 {
		return value, nil
	}

	increment := alignment - remainder

	if value > ^uint64(0)-increment {
		return 0, fmt.Errorf(
			"aligned value overflows",
		)
	}

	return value + increment, nil
}

func containsNonZero(data []byte) bool {
	for _, value := range data {
		if value != 0 {
			return true
		}
	}

	return false
}
