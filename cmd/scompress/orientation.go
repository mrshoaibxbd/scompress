package main

import (
	"bufio"
	"encoding/binary"
	"image"
	"image/color"
	"io"
	"os"
)

// readJPEGOrientation reads only JPEG marker headers and the small EXIF APP1
// segment; it does not load the whole photo into memory a second time.
func readJPEGOrientation(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 1
	}
	defer f.Close()
	r := bufio.NewReader(f)
	var soi [2]byte
	if _, err := io.ReadFull(r, soi[:]); err != nil || soi != [2]byte{0xff, 0xd8} {
		return 1
	}
	for {
		b, err := r.ReadByte()
		if err != nil {
			return 1
		}
		if b != 0xff {
			continue
		}
		for b == 0xff {
			b, err = r.ReadByte()
			if err != nil {
				return 1
			}
		}
		if b == 0xd9 || b == 0xda {
			return 1
		} // EOI or start of scan.
		if b == 0xd8 || b == 0x01 || (b >= 0xd0 && b <= 0xd7) {
			continue // Standalone markers have no length field.
		}
		var lengthBytes [2]byte
		if _, err := io.ReadFull(r, lengthBytes[:]); err != nil {
			return 1
		}
		segmentLength := int(binary.BigEndian.Uint16(lengthBytes[:]))
		if segmentLength < 2 {
			return 1
		}
		payloadLength := segmentLength - 2
		if b == 0xe1 && payloadLength <= 65533 {
			payload := make([]byte, payloadLength)
			if _, err := io.ReadFull(r, payload); err != nil {
				return 1
			}
			if orientation := parseEXIFOrientation(payload); orientation >= 1 && orientation <= 8 {
				return orientation
			}
		} else {
			if _, err := io.CopyN(io.Discard, r, int64(payloadLength)); err != nil {
				return 1
			}
		}
	}
}

func parseEXIFOrientation(payload []byte) int {
	if len(payload) < 14 || string(payload[:6]) != "Exif\x00\x00" {
		return 1
	}
	tiff := payload[6:]
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 1
	}
	if order.Uint16(tiff[2:4]) != 42 {
		return 1
	}
	offset := int(order.Uint32(tiff[4:8]))
	if offset < 8 || offset+2 > len(tiff) {
		return 1
	}
	count := int(order.Uint16(tiff[offset : offset+2]))
	entry := offset + 2
	for i := 0; i < count; i++ {
		pos := entry + i*12
		if pos+12 > len(tiff) {
			return 1
		}
		tag := order.Uint16(tiff[pos : pos+2])
		typeID := order.Uint16(tiff[pos+2 : pos+4])
		valueCount := order.Uint32(tiff[pos+4 : pos+8])
		if tag == 0x0112 && typeID == 3 && valueCount >= 1 {
			value := int(order.Uint16(tiff[pos+8 : pos+10]))
			if value >= 1 && value <= 8 {
				return value
			}
		}
	}
	return 1
}

func applyOrientation(src image.Image, orientation int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dw, dh := w, h
	if orientation >= 5 && orientation <= 8 {
		dw, dh = h, w
	}
	dst := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var dx, dy int
			switch orientation {
			case 2:
				dx, dy = w-1-x, y
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dx, dy = x, h-1-y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			default:
				dx, dy = x, y
			}
			dst.SetNRGBA(dx, dy, color.NRGBAModel.Convert(src.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA))
		}
	}
	return dst
}
