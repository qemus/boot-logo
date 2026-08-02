package main

import (
	"image"
	"image/color"
	"testing"
)

func TestEncodeBitmapCompositesTransparencyOnBlack(
	t *testing.T,
) {
	source := image.NewNRGBA(
		image.Rect(0, 0, 3, 1),
	)

	source.SetNRGBA(
		0,
		0,
		color.NRGBA{
			R: 0xff,
			G: 0x80,
			B: 0x40,
			A: 0x00,
		},
	)
	source.SetNRGBA(
		1,
		0,
		color.NRGBA{
			R: 0xff,
			A: 0x80,
		},
	)
	source.SetNRGBA(
		2,
		0,
		color.NRGBA{
			R: 0xff,
			G: 0x80,
			B: 0x40,
			A: 0xff,
		},
	)

	bitmap, err := encodeBitmap(source)
	if err != nil {
		t.Fatalf("encodeBitmap() returned an error: %v", err)
	}

	decoded, err := decodeBitmap(bitmap)
	if err != nil {
		t.Fatalf("decodeBitmap() returned an error: %v", err)
	}

	assertPixelEquals(
		t,
		decoded,
		0,
		0,
		color.NRGBA{
			A: 0xff,
		},
	)
	assertPixelEquals(
		t,
		decoded,
		1,
		0,
		color.NRGBA{
			R: 0x80,
			A: 0xff,
		},
	)
	assertPixelEquals(
		t,
		decoded,
		2,
		0,
		color.NRGBA{
			R: 0xff,
			G: 0x80,
			B: 0x40,
			A: 0xff,
		},
	)
}

func assertPixelEquals(
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
		t.Fatalf(
			"pixel (%d,%d) = %#v, want %#v",
			x,
			y,
			actual,
			expected,
		)
	}
}
