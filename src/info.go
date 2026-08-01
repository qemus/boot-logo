package main

import (
	"encoding/json"
	"fmt"
	"image"
	"io"
	"os"

	"github.com/linuxboot/fiano/pkg/uefi"
)

type firmwareInfo struct {
	Path                 string `json:"path"`
	FirmwareType         string `json:"firmware_type"`
	FirmwareSize         int64  `json:"firmware_size"`
	LogoFileGUID         string `json:"logo_file_guid"`
	ImageFormat          string `json:"image_format"`
	ImageWidth           int    `json:"image_width"`
	ImageHeight          int    `json:"image_height"`
	ImageDepth           int    `json:"image_depth"`
	EmbeddedImageSize    int    `json:"embedded_image_size"`
	ReplacementSupported bool   `json:"replacement_supported"`
}

func inspectFirmware(path string) (firmwareInfo, error) {
	info := firmwareInfo{
		Path:         path,
		LogoFileGUID: logoFileGUID.String(),
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		return firmwareInfo{}, fmt.Errorf(
			"stat firmware %q: %w",
			path,
			err,
		)
	}

	info.FirmwareSize = fileInfo.Size()

	firmware, err := readFirmware(path)
	if err != nil {
		return firmwareInfo{}, err
	}

	info.FirmwareType = firmwareType(firmware)

	match, err := findBootLogo(firmware)
	if err != nil {
		return firmwareInfo{}, err
	}

	peImage, err := sectionPayload(match.section)
	if err != nil {
		return firmwareInfo{}, err
	}

	source, err := decodeHIIImage(
		peImage,
		match.location,
	)
	if err != nil {
		return firmwareInfo{}, fmt.Errorf(
			"decode HII boot logo: %w",
			err,
		)
	}

	bounds := source.Bounds()

	info.ImageFormat = hiiImageFormat(
		match.location.blockType,
	)
	info.ImageWidth = bounds.Dx()
	info.ImageHeight = bounds.Dy()
	info.ImageDepth = hiiImageDepth(
		match.location.blockType,
		source,
	)
	info.EmbeddedImageSize = match.location.blockLength

	if _, err := encodeBitmap(source); err != nil {
		return firmwareInfo{}, fmt.Errorf(
			"validate replacement conversion: %w",
			err,
		)
	}

	info.ReplacementSupported = true

	return info, nil
}

func verifyFirmware(path string) error {
	_, err := inspectFirmware(path)
	return err
}

func printFirmwareInfo(
	writer io.Writer,
	info firmwareInfo,
	asJSON bool,
) error {
	if asJSON {
		encoder := json.NewEncoder(writer)
		encoder.SetIndent("", "  ")

		if err := encoder.Encode(info); err != nil {
			return fmt.Errorf(
				"encode firmware information: %w",
				err,
			)
		}

		return nil
	}

	fmt.Fprintf(writer, "Firmware: %s\n", info.Path)
	fmt.Fprintf(writer, "Firmware type: %s\n", info.FirmwareType)
	fmt.Fprintf(writer, "Firmware size: %d bytes\n", info.FirmwareSize)
	fmt.Fprintf(writer, "LogoDxe GUID: %s\n", info.LogoFileGUID)
	fmt.Fprintf(writer, "Image format: %s\n", info.ImageFormat)
	fmt.Fprintf(
		writer,
		"Image dimensions: %dx%d\n",
		info.ImageWidth,
		info.ImageHeight,
	)
	fmt.Fprintf(writer, "Image depth: %d bits\n", info.ImageDepth)
	fmt.Fprintf(
		writer,
		"Embedded image size: %d bytes\n",
		info.EmbeddedImageSize,
	)
	fmt.Fprintf(
		writer,
		"Replacement supported: %t\n",
		info.ReplacementSupported,
	)

	return nil
}

func firmwareType(firmware uefi.Firmware) string {
	if _, ok := firmware.(*uefi.File); ok {
		return "standalone FFS"
	}

	return "UEFI firmware"
}

func hiiImageFormat(blockType byte) string {
	switch blockType {
	case hiiImage1Bit, hiiImage1BitTrans:
		return "HII 1-bit bitmap"

	case hiiImage4Bit, hiiImage4BitTrans:
		return "HII 4-bit bitmap"

	case hiiImage8Bit, hiiImage8BitTrans:
		return "HII 8-bit bitmap"

	case hiiImage24Bit, hiiImage24BitTrans:
		return "HII 24-bit bitmap"

	case hiiImageJPEG:
		return "JPEG"

	case hiiImagePNG:
		return "PNG"

	default:
		return fmt.Sprintf(
			"unknown HII image block 0x%02x",
			blockType,
		)
	}
}

func hiiImageDepth(
	blockType byte,
	source image.Image,
) int {
	switch blockType {
	case hiiImage1Bit, hiiImage1BitTrans:
		return 1

	case hiiImage4Bit, hiiImage4BitTrans:
		return 4

	case hiiImage8Bit, hiiImage8BitTrans:
		return 8

	case hiiImage24Bit, hiiImage24BitTrans, hiiImageJPEG:
		return 24

	case hiiImagePNG:
		return imageDepth(source)

	default:
		return imageDepth(source)
	}
}

func imageDepth(source image.Image) int {
	switch source.(type) {
	case *image.NRGBA, *image.RGBA, *image.NRGBA64, *image.RGBA64:
		return 32
	case *image.Gray, *image.Gray16, *image.Paletted:
		return 8
	default:
		// Best-effort fallback for formats like JPEG (*image.YCbCr).
		return 24
	}
}
