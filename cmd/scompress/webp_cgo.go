package main

/*
#cgo LDFLAGS: -static -lwebp -lsharpyuv -lm
#include <stdint.h>
#include <stddef.h>
#include <webp/decode.h>
#include <webp/encode.h>

static int sc_webp_features(const uint8_t* data, size_t size,
                            int* width, int* height, int* animated) {
    WebPBitstreamFeatures features;
    VP8StatusCode status = WebPGetFeatures(data, size, &features);
    if (status != VP8_STATUS_OK) return (int)status;
    *width = features.width;
    *height = features.height;
    *animated = features.has_animation;
    return 0;
}

static uint8_t* sc_webp_decode(const uint8_t* data, size_t size,
                               int* width, int* height) {
    return WebPDecodeRGBA(data, size, width, height);
}

static size_t sc_webp_encode(const uint8_t* rgba, int width, int height,
                             int stride, float quality, uint8_t** output) {
    return WebPEncodeRGBA(rgba, width, height, stride, quality, output);
}
*/
import "C"

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"os"
	"unsafe"
)

func decodeWebPFile(path string) (image.Image, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false, err
	}
	if len(data) == 0 {
		return nil, false, errors.New("empty file")
	}
	width, height, animated, err := webPFeatures(data)
	if err != nil {
		return nil, false, err
	}
	if animated {
		return nil, true, nil
	}
	if width < 1 || height < 1 || int64(width)*int64(height) > 100_000_000 {
		return nil, false, errors.New("invalid or excessively large WebP dimensions")
	}
	var cWidth, cHeight C.int
	pixels := C.sc_webp_decode((*C.uint8_t)(unsafe.Pointer(&data[0])), C.size_t(len(data)), &cWidth, &cHeight)
	if pixels == nil {
		return nil, false, errors.New("WebP decoder could not decode the image")
	}
	defer C.WebPFree(unsafe.Pointer(pixels))
	width, height = int(cWidth), int(cHeight)
	if width < 1 || height < 1 || int64(width)*int64(height) > 100_000_000 {
		return nil, false, errors.New("invalid WebP decoder dimensions")
	}
	length := width * height * 4
	if uint64(length) > uint64(^uint32(0)>>1) {
		return nil, false, errors.New("WebP image buffer is too large for this build")
	}
	bytes := C.GoBytes(unsafe.Pointer(pixels), C.int(length))
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	copy(img.Pix, bytes)
	return img, false, nil
}

func webPFeatures(data []byte) (int, int, bool, error) {
	if len(data) == 0 {
		return 0, 0, false, errors.New("empty file")
	}
	var width, height, animated C.int
	status := C.sc_webp_features((*C.uint8_t)(unsafe.Pointer(&data[0])), C.size_t(len(data)), &width, &height, &animated)
	if status != 0 {
		return 0, 0, false, fmt.Errorf("invalid WebP header (decoder status %d)", int(status))
	}
	return int(width), int(height), animated != 0, nil
}

func encodeWebP(img image.Image, quality int) ([]byte, error) {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width < 1 || height < 1 {
		return nil, errors.New("cannot encode an empty image")
	}
	if int64(width)*int64(height) > 100_000_000 {
		return nil, errors.New("image dimensions exceed the 100-megapixel safety limit")
	}
	rgba := make([]byte, width*height*4)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			c := color.NRGBAModel.Convert(img.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA)
			i := (y*width + x) * 4
			rgba[i], rgba[i+1], rgba[i+2], rgba[i+3] = c.R, c.G, c.B, c.A
		}
	}
	var output *C.uint8_t
	size := C.sc_webp_encode((*C.uint8_t)(unsafe.Pointer(&rgba[0])), C.int(width), C.int(height), C.int(width*4), C.float(quality), &output)
	if size == 0 || output == nil {
		return nil, errors.New("WebP encoder failed")
	}
	defer C.WebPFree(unsafe.Pointer(output))
	if uint64(size) > uint64(^uint32(0)>>1) {
		return nil, errors.New("WebP output is too large for this build")
	}
	return C.GoBytes(unsafe.Pointer(output), C.int(size)), nil
}
