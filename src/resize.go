package main

import (
	"errors"
	"fmt"
	"image"
	"math"

	"github.com/linuxboot/fiano/pkg/uefi"
	"golang.org/x/image/draw"
)

type logoResizeInfo struct {
	originalWidth  int
	originalHeight int
	resizedWidth   int
	resizedHeight  int
}

type plannedLogoReplacement struct {
	image      image.Image
	peImage    []byte
	resizeInfo *logoResizeInfo
}

func planBootLogoReplacement(
	firmware uefi.Firmware,
	match logoMatch,
	peImage []byte,
	source image.Image,
) (plannedLogoReplacement, error) {
	layout, err := findBootLogoLayout(
		firmware,
		match.section,
	)
	if err != nil {
		return plannedLogoReplacement{}, err
	}

	updatedPE, replaceErr := replaceHIIImage(
		peImage,
		match.location,
		source,
	)
	if replaceErr == nil {
		fits, err := bootLogoLayoutFits(
			layout,
			updatedPE,
		)
		if err != nil {
			return plannedLogoReplacement{}, err
		}

		if fits {
			return plannedLogoReplacement{
				image:   source,
				peImage: updatedPE,
			}, nil
		}
	} else if !errors.Is(
		replaceErr,
		errHIIImagePackageTooLarge,
	) {
		return plannedLogoReplacement{}, replaceErr
	}

	_, width, height, err := hiiImageDimensions(source)
	if err != nil {
		return plannedLogoReplacement{}, err
	}

	var planned plannedLogoReplacement

	if bootLogoPathUsesCompression(layout.sections) {
		planned, err = planCompressedBootLogoResize(
			layout,
			peImage,
			match.location,
			source,
			width,
			height,
		)
	} else {
		planned, err = planUncompressedBootLogoResize(
			layout,
			peImage,
			match.location,
			source,
			width,
			height,
		)
	}
	if err != nil {
		return plannedLogoReplacement{}, err
	}

	planned.resizeInfo = &logoResizeInfo{
		originalWidth:  width,
		originalHeight: height,
		resizedWidth:   planned.image.Bounds().Dx(),
		resizedHeight:  planned.image.Bounds().Dy(),
	}

	return planned, nil
}

func planUncompressedBootLogoResize(
	layout bootLogoLayout,
	peImage []byte,
	location hiiImageLocation,
	source image.Image,
	originalWidth int,
	originalHeight int,
) (plannedLogoReplacement, error) {
	maximumWidth, err := maximumHII24BitWidth(
		location,
		originalWidth,
		originalHeight,
	)
	if err != nil {
		return plannedLogoReplacement{}, err
	}

	bestWidth := 0
	bestHeight := 0

	low := 1
	high := maximumWidth

	for low <= high {
		width := low + (high-low)/2
		height := proportionalHeight(
			originalWidth,
			originalHeight,
			width,
		)

		block, err := newHII24BitBlock(
			width,
			height,
		)
		if err != nil {
			return plannedLogoReplacement{}, err
		}

		candidatePE, err := replaceHIIImageBlock(
			peImage,
			location,
			block,
			nil,
		)
		if errors.Is(err, errHIIImagePackageTooLarge) {
			high = width - 1
			continue
		}
		if err != nil {
			return plannedLogoReplacement{}, err
		}

		fits, err := bootLogoLayoutFits(
			layout,
			candidatePE,
		)
		if err != nil {
			return plannedLogoReplacement{}, err
		}

		if fits {
			bestWidth = width
			bestHeight = height
			low = width + 1
		} else {
			high = width - 1
		}
	}

	if bestWidth == 0 || bestHeight == 0 {
		return plannedLogoReplacement{}, fmt.Errorf(
			"boot logo cannot fit in the containing firmware volume",
		)
	}

	resized := resizeBootLogo(
		source,
		bestWidth,
		bestHeight,
	)

	updatedPE, err := replaceHII24BitImage(
		peImage,
		location,
		resized,
	)
	if err != nil {
		return plannedLogoReplacement{}, err
	}

	fits, err := bootLogoLayoutFits(
		layout,
		updatedPE,
	)
	if err != nil {
		return plannedLogoReplacement{}, err
	}
	if !fits {
		return plannedLogoReplacement{}, fmt.Errorf(
			"boot logo size planner produced a layout that exceeds the containing firmware volume",
		)
	}

	return plannedLogoReplacement{
		image:   resized,
		peImage: updatedPE,
	}, nil
}

func planCompressedBootLogoResize(
	layout bootLogoLayout,
	peImage []byte,
	location hiiImageLocation,
	source image.Image,
	originalWidth int,
	originalHeight int,
) (plannedLogoReplacement, error) {
	maximumWidth, err := maximumHII24BitWidth(
		location,
		originalWidth,
		originalHeight,
	)
	if err != nil {
		return plannedLogoReplacement{}, err
	}

	low := 1
	high := maximumWidth

	var best plannedLogoReplacement

	for low <= high {
		width := low + (high-low)/2
		height := proportionalHeight(
			originalWidth,
			originalHeight,
			width,
		)

		resized := resizeBootLogo(
			source,
			width,
			height,
		)

		candidatePE, err := replaceHII24BitImage(
			peImage,
			location,
			resized,
		)
		if errors.Is(err, errHIIImagePackageTooLarge) {
			high = width - 1
			continue
		}
		if err != nil {
			return plannedLogoReplacement{}, err
		}

		fits, err := bootLogoLayoutFits(
			layout,
			candidatePE,
		)
		if err != nil {
			return plannedLogoReplacement{}, err
		}

		if fits {
			best = plannedLogoReplacement{
				image:   resized,
				peImage: candidatePE,
			}
			low = width + 1
		} else {
			high = width - 1
		}
	}

	if best.image == nil {
		return plannedLogoReplacement{}, fmt.Errorf(
			"boot logo cannot fit in the containing firmware volume",
		)
	}

	return best, nil
}

func maximumHII24BitWidth(
	location hiiImageLocation,
	originalWidth int,
	originalHeight int,
) (int, error) {
	if originalWidth <= 0 || originalHeight <= 0 {
		return 0, fmt.Errorf(
			"image dimensions must be greater than zero",
		)
	}

	packageOverhead := location.packageLength -
		location.blockLength
	maximumBlockLength := maxUint24Value - packageOverhead

	if maximumBlockLength <= 5 {
		return 0, fmt.Errorf(
			"HII image package has no space for image pixels",
		)
	}

	maximumPixels := (maximumBlockLength - 5) / 3
	requestedPixels := uint64(originalWidth) *
		uint64(originalHeight)

	if requestedPixels <= uint64(maximumPixels) {
		return originalWidth, nil
	}

	scale := math.Sqrt(
		float64(maximumPixels) /
			float64(requestedPixels),
	)

	maximumWidth := int(
		math.Floor(float64(originalWidth) * scale),
	)

	if maximumWidth < 1 {
		maximumWidth = 1
	}
	if maximumWidth > originalWidth {
		maximumWidth = originalWidth
	}

	for maximumWidth > 1 {
		height := proportionalHeight(
			originalWidth,
			originalHeight,
			maximumWidth,
		)

		pixels := uint64(maximumWidth) * uint64(height)
		if pixels <= uint64(maximumPixels) {
			break
		}

		maximumWidth--
	}

	return maximumWidth, nil
}

func proportionalHeight(
	originalWidth int,
	originalHeight int,
	width int,
) int {
	height := int(
		uint64(originalHeight) *
			uint64(width) /
			uint64(originalWidth),
	)

	if height < 1 {
		height = 1
	}

	return height
}

func resizeBootLogo(
	source image.Image,
	width int,
	height int,
) *image.NRGBA {
	output := image.NewNRGBA(
		image.Rect(0, 0, width, height),
	)

	draw.CatmullRom.Scale(
		output,
		output.Bounds(),
		source,
		source.Bounds(),
		draw.Src,
		nil,
	)

	return output
}

func replaceHII24BitImage(
	data []byte,
	location hiiImageLocation,
	source image.Image,
) ([]byte, error) {
	block, err := encodeHII24BitImage(source)
	if err != nil {
		return nil, err
	}

	return replaceHIIImageBlock(
		data,
		location,
		block,
		nil,
	)
}

func newHII24BitBlock(
	width int,
	height int,
) ([]byte, error) {
	if width <= 0 || height <= 0 ||
		width > maxUint16Value ||
		height > maxUint16Value {
		return nil, fmt.Errorf(
			"invalid HII image dimensions: %dx%d",
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
	block[1] = byte(width)
	block[2] = byte(width >> 8)
	block[3] = byte(height)
	block[4] = byte(height >> 8)

	return block, nil
}
