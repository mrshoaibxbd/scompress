package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
)

type pngMetadataChunk struct {
	typ string
	raw []byte
}

func imageMetadata(path, format string, mode metadataMode) ([]byte, error) {
	if mode == metadataNone {
		return nil, nil
	}
	switch format {
	case "jpeg":
		return readJPEGMetadata(path, mode == metadataKeep)
	case "png":
		return readPNGMetadata(path, mode == metadataKeep)
	case "gif", "webp":
		if mode == metadataKeep {
			return nil, fmt.Errorf("--keep-metadata is not available for %s yet; use the default privacy mode or --no-metadata", format)
		}
		return nil, nil
	default:
		return nil, nil
	}
}

// readJPEGMetadata extracts metadata markers before the first scan. The default
// privacy mode keeps only embedded ICC color profiles; --keep-metadata also
// retains EXIF, XMP, IPTC and comments. EXIF orientation is normalized because
// the decoded pixels have already been rotated into their displayed orientation.
func readJPEGMetadata(path string, keepAll bool) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) < 4 || data[0] != 0xff || data[1] != 0xd8 {
		return nil, errors.New("invalid JPEG header while reading metadata")
	}
	var out []byte
	for pos := 2; pos < len(data); {
		if data[pos] != 0xff {
			break
		}
		start := pos
		for pos < len(data) && data[pos] == 0xff {
			pos++
		}
		if pos >= len(data) {
			break
		}
		marker := data[pos]
		pos++
		if marker == 0xda || marker == 0xd9 { // SOS or EOI
			break
		}
		if marker == 0x01 || (marker >= 0xd0 && marker <= 0xd7) {
			continue
		}
		if pos+2 > len(data) {
			break
		}
		segmentLength := int(binary.BigEndian.Uint16(data[pos : pos+2]))
		if segmentLength < 2 || pos+segmentLength > len(data) {
			break
		}
		end := pos + segmentLength
		payload := data[pos+2 : end]
		keep := false
		switch marker {
		case 0xe2: // APP2: ICC profile
			keep = keepAll || hasPrefix(payload, []byte("ICC_PROFILE\x00"))
		case 0xe1, 0xed, 0xfe: // EXIF/XMP, IPTC, and comments
			keep = keepAll
		}
		if keep {
			segment := append([]byte(nil), data[start:end]...)
			if marker == 0xe1 {
				normalizeEXIFOrientationMarker(segment)
			}
			out = append(out, segment...)
		}
		pos = end
	}
	return out, nil
}

func normalizeEXIFOrientationMarker(segment []byte) {
	// JPEG marker (2 bytes), segment length (2 bytes), then EXIF payload.
	if len(segment) < 18 || segment[0] != 0xff || segment[1] != 0xe1 {
		return
	}
	payload := segment[4:]
	if len(payload) < 14 || string(payload[:6]) != "Exif\x00\x00" {
		return
	}
	tiff := payload[6:]
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return
	}
	if order.Uint16(tiff[2:4]) != 42 {
		return
	}
	offset := int(order.Uint32(tiff[4:8]))
	if offset < 8 || offset+2 > len(tiff) {
		return
	}
	count := int(order.Uint16(tiff[offset : offset+2]))
	entry := offset + 2
	for i := 0; i < count; i++ {
		pos := entry + i*12
		if pos+12 > len(tiff) {
			return
		}
		tag := order.Uint16(tiff[pos : pos+2])
		typeID := order.Uint16(tiff[pos+2 : pos+4])
		valueCount := order.Uint32(tiff[pos+4 : pos+8])
		if tag == 0x0112 && typeID == 3 && valueCount >= 1 {
			order.PutUint16(tiff[pos+8:pos+10], 1)
			return
		}
	}
}

func insertJPEGMetadata(encoded, metadata []byte) ([]byte, error) {
	if len(metadata) == 0 {
		return encoded, nil
	}
	if len(encoded) < 2 || encoded[0] != 0xff || encoded[1] != 0xd8 {
		return nil, errors.New("cannot insert metadata into invalid JPEG output")
	}
	insertAt := 2
	// jpeg.Encode writes a JFIF APP0 marker immediately after SOI. Keep that
	// marker in its conventional position, then add the retained metadata.
	if len(encoded) >= 6 && encoded[2] == 0xff && encoded[3] == 0xe0 {
		segmentLength := int(binary.BigEndian.Uint16(encoded[4:6]))
		end := 4 + segmentLength
		if segmentLength >= 2 && end <= len(encoded) {
			insertAt = end
		}
	}
	out := make([]byte, 0, len(encoded)+len(metadata))
	out = append(out, encoded[:insertAt]...)
	out = append(out, metadata...)
	out = append(out, encoded[insertAt:]...)
	return out, nil
}

func readPNGMetadata(path string, keepAll bool) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	chunks, err := parsePNGChunks(data)
	if err != nil {
		return nil, fmt.Errorf("read PNG metadata: %w", err)
	}
	var out []byte
	for _, chunk := range chunks {
		keep := false
		switch chunk.typ {
		case "cHRM", "gAMA", "iCCP", "sBIT", "sRGB", "pHYs":
			keep = true
		case "eXIf", "tEXt", "zTXt", "iTXt":
			keep = keepAll
		}
		if keep {
			out = append(out, chunk.raw...)
		}
	}
	return out, nil
}

func parsePNGChunks(data []byte) ([]pngMetadataChunk, error) {
	return parsePNGChunkSequence(data, true)
}

func parsePNGChunkSequence(data []byte, requireIHDR bool) ([]pngMetadataChunk, error) {
	const signature = "\x89PNG\r\n\x1a\n"
	if len(data) < 8 || string(data[:8]) != signature {
		return nil, errors.New("invalid PNG signature")
	}
	var chunks []pngMetadataChunk
	for pos := 8; pos+12 <= len(data); {
		length64 := uint64(binary.BigEndian.Uint32(data[pos : pos+4]))
		if length64 > uint64(len(data)) || uint64(pos)+12+length64 > uint64(len(data)) {
			return nil, errors.New("invalid PNG chunk length")
		}
		length := int(length64)
		typ := string(data[pos+4 : pos+8])
		end := pos + 12 + length
		raw := append([]byte(nil), data[pos:end]...)
		chunks = append(chunks, pngMetadataChunk{typ: typ, raw: raw})
		pos = end
		if typ == "IEND" {
			break
		}
	}
	if requireIHDR && (len(chunks) == 0 || chunks[0].typ != "IHDR") {
		return nil, errors.New("PNG is missing IHDR")
	}
	return chunks, nil
}

func insertPNGMetadata(encoded, metadata []byte) ([]byte, error) {
	if len(metadata) == 0 {
		return encoded, nil
	}
	metadataChunks, err := parsePNGChunkSequence(append([]byte("\x89PNG\r\n\x1a\n"), metadata...), false)
	if err != nil {
		return nil, fmt.Errorf("invalid saved PNG metadata: %w", err)
	}
	outputChunks, err := parsePNGChunks(encoded)
	if err != nil {
		return nil, fmt.Errorf("cannot insert metadata into PNG output: %w", err)
	}
	var beforePalette, beforeIDAT [][]byte
	for _, chunk := range metadataChunks {
		switch chunk.typ {
		case "cHRM", "gAMA", "iCCP", "sBIT", "sRGB":
			beforePalette = append(beforePalette, chunk.raw)
		default:
			beforeIDAT = append(beforeIDAT, chunk.raw)
		}
	}

	var out []byte
	out = append(out, []byte("\x89PNG\r\n\x1a\n")...)
	insertedPalette := false
	insertedBeforeIDAT := false
	for _, chunk := range outputChunks {
		if chunk.typ == "IHDR" {
			out = append(out, chunk.raw...)
			if !insertedPalette {
				for _, raw := range beforePalette {
					out = append(out, raw...)
				}
				insertedPalette = true
			}
			continue
		}
		if chunk.typ == "IDAT" && !insertedBeforeIDAT {
			for _, raw := range beforeIDAT {
				out = append(out, raw...)
			}
			insertedBeforeIDAT = true
		}
		out = append(out, chunk.raw...)
	}
	if !insertedBeforeIDAT {
		return nil, errors.New("PNG output has no IDAT chunk")
	}
	return out, nil
}

// makePNGChunk is useful in tests and validates the CRC of preserved chunk data.
func makePNGChunk(typ string, payload []byte) []byte {
	chunk := make([]byte, 12+len(payload))
	binary.BigEndian.PutUint32(chunk[:4], uint32(len(payload)))
	copy(chunk[4:8], typ)
	copy(chunk[8:8+len(payload)], payload)
	binary.BigEndian.PutUint32(chunk[8+len(payload):], crc32.ChecksumIEEE(chunk[4:8+len(payload)]))
	return chunk
}

func hasPrefix(data, prefix []byte) bool {
	if len(data) < len(prefix) {
		return false
	}
	for i := range prefix {
		if data[i] != prefix[i] {
			return false
		}
	}
	return true
}
