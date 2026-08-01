package main

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type fixtureImageFormat struct {
	name      string
	extension string
	encode    func(io.Writer, image.Image) error
	decode    func(io.Reader) (image.Image, error)
}

var fixtureImageFormats = []fixtureImageFormat{
	{
		name:      "BMP",
		extension: ".bmp",
		encode: func(
			writer io.Writer,
			source image.Image,
		) error {
			data, err := encodeBitmap(source)
			if err != nil {
				return err
			}

			_, err = writer.Write(data)

			return err
		},
		decode: func(
			reader io.Reader,
		) (image.Image, error) {
			data, err := io.ReadAll(reader)
			if err != nil {
				return nil, err
			}

			return decodeBitmap(data)
		},
	},
	{
		name:      "PNG",
		extension: ".png",
		encode: func(
			writer io.Writer,
			source image.Image,
		) error {
			return png.Encode(writer, source)
		},
		decode: png.Decode,
	},
	{
		name:      "JPEG",
		extension: ".jpg",
		encode: func(
			writer io.Writer,
			source image.Image,
		) error {
			return jpeg.Encode(
				writer,
				source,
				&jpeg.Options{
					Quality: 95,
				},
			)
		},
		decode: jpeg.Decode,
	},
}

func TestExtractBootLogoFixturesMatchReference(
	t *testing.T,
) {
	expected := readFixture(
		t,
		"test.bmp",
	)

	for _, fixture := range firmwareFixtures {
		fixture := fixture

		t.Run(fixture.name, func(t *testing.T) {
			outputPath := filepath.Join(
				t.TempDir(),
				"extracted.bmp",
			)

			if err := extractBootLogo(
				fixturePath(fixture.file),
				outputPath,
			); err != nil {
				t.Fatalf(
					"extractBootLogo() returned an error: %v",
					err,
				)
			}

			actual, err := os.ReadFile(outputPath)
			if err != nil {
				t.Fatalf(
					"read extracted bitmap: %v",
					err,
				)
			}

			if !bytes.Equal(actual, expected) {
				t.Fatal(
					"extracted bitmap does not match tests/test.bmp",
				)
			}
		})
	}
}

func TestReplaceBootLogoFixturesFromSupportedFormats(
	t *testing.T,
) {
	referenceData := readFixture(
		t,
		"test.bmp",
	)

	reference, err := decodeBitmap(referenceData)
	if err != nil {
		t.Fatalf(
			"decode tests/test.bmp: %v",
			err,
		)
	}

	mirrored := mirrorFixtureImageHorizontally(
		reference,
	)

	for _, format := range fixtureImageFormats {
		format := format

		t.Run(format.name, func(t *testing.T) {
			inputData := encodeFixtureImage(
				t,
				format,
				mirrored,
			)

			expectedBMP := decodeFixtureImageToBitmap(
				t,
				format,
				inputData,
			)

			if bytes.Equal(
				expectedBMP,
				referenceData,
			) {
				t.Fatal(
					"transformed replacement unexpectedly matches the original logo",
				)
			}

			for _, fixture := range firmwareFixtures {
				fixture := fixture

				t.Run(
					fixture.name,
					func(t *testing.T) {
						testReplacementFormat(
							t,
							fixture.file,
							format.extension,
							inputData,
							expectedBMP,
						)
					},
				)
			}
		})
	}
}

func testReplacementFormat(
	t *testing.T,
	fixtureName string,
	extension string,
	inputData []byte,
	expectedBMP []byte,
) {
	t.Helper()

	directory := t.TempDir()

	firmwarePath := filepath.Join(
		directory,
		fixtureName,
	)

	originalFirmware := copyFixture(
		t,
		fixtureName,
		firmwarePath,
		0o644,
	)

	imagePath := filepath.Join(
		directory,
		"replacement"+extension,
	)

	if err := os.WriteFile(
		imagePath,
		inputData,
		0o644,
	); err != nil {
		t.Fatalf(
			"write replacement image: %v",
			err,
		)
	}

	if err := replaceBootLogo(
		imagePath,
		firmwarePath,
		firmwarePath,
	); err != nil {
		t.Fatalf(
			"replaceBootLogo() returned an error: %v",
			err,
		)
	}

	modifiedFirmware, err := os.ReadFile(
		firmwarePath,
	)
	if err != nil {
		t.Fatalf(
			"read modified firmware: %v",
			err,
		)
	}

	if bytes.Equal(
		modifiedFirmware,
		originalFirmware,
	) {
		t.Fatal(
			"replacement did not modify the firmware",
		)
	}

	extractedPath := filepath.Join(
		directory,
		"extracted.bmp",
	)

	if err := extractBootLogo(
		firmwarePath,
		extractedPath,
	); err != nil {
		t.Fatalf(
			"extract modified boot logo: %v",
			err,
		)
	}

	extractedBMP, err := os.ReadFile(
		extractedPath,
	)
	if err != nil {
		t.Fatalf(
			"read extracted bitmap: %v",
			err,
		)
	}

	if !bytes.Equal(
		extractedBMP,
		expectedBMP,
	) {
		t.Fatal(
			"extracted logo does not match the decoded replacement image",
		)
	}
}

func encodeFixtureImage(
	t *testing.T,
	format fixtureImageFormat,
	source image.Image,
) []byte {
	t.Helper()

	var output bytes.Buffer

	if err := format.encode(
		&output,
		source,
	); err != nil {
		t.Fatalf(
			"encode %s replacement: %v",
			format.name,
			err,
		)
	}

	if output.Len() == 0 {
		t.Fatalf(
			"encoded %s replacement is empty",
			format.name,
		)
	}

	return output.Bytes()
}

func decodeFixtureImageToBitmap(
	t *testing.T,
	format fixtureImageFormat,
	data []byte,
) []byte {
	t.Helper()

	decoded, err := format.decode(
		bytes.NewReader(data),
	)
	if err != nil {
		t.Fatalf(
			"independently decode %s replacement: %v",
			format.name,
			err,
		)
	}

	bitmap, err := encodeBitmap(decoded)
	if err != nil {
		t.Fatalf(
			"encode expected %s bitmap: %v",
			format.name,
			err,
		)
	}

	return bitmap
}

func mirrorFixtureImageHorizontally(
	source image.Image,
) *image.NRGBA {
	bounds := source.Bounds()

	output := image.NewNRGBA(
		image.Rect(
			0,
			0,
			bounds.Dx(),
			bounds.Dy(),
		),
	)

	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			sourceX :=
				bounds.Max.X - 1 - x
			sourceY :=
				bounds.Min.Y + y

			output.Set(
				x,
				y,
				source.At(sourceX, sourceY),
			)
		}
	}

	return output
}
