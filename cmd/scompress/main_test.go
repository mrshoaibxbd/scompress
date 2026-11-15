package main

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseArgsCompactTarget(t *testing.T) {
	opts, err := parseArgs([]string{"photo.png", "--500"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.targetBytes != 500_000 {
		t.Fatalf("targetBytes = %d, want 500000", opts.targetBytes)
	}
	if len(opts.inputs) != 1 || opts.inputs[0] != "photo.png" {
		t.Fatalf("unexpected inputs: %#v", opts.inputs)
	}
}

func TestParseArgsLongTarget(t *testing.T) {
	opts, err := parseArgs([]string{"--target=250", "photo.jpg"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.targetBytes != 250_000 {
		t.Fatalf("targetBytes = %d, want 250000", opts.targetBytes)
	}
}

func TestApplyOrientationSixRotatesClockwise(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	src.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	src.SetNRGBA(1, 0, color.NRGBA{G: 255, A: 255})
	got := applyOrientation(src, 6)
	if got.Bounds().Dx() != 1 || got.Bounds().Dy() != 2 {
		t.Fatalf("bounds = %v, want 1x2", got.Bounds())
	}
	if c := color.NRGBAModel.Convert(got.At(0, 0)).(color.NRGBA); c.R != 255 {
		t.Fatalf("top pixel = %#v, want red", c)
	}
	if c := color.NRGBAModel.Convert(got.At(0, 1)).(color.NRGBA); c.G != 255 {
		t.Fatalf("bottom pixel = %#v, want green", c)
	}
}

func TestParseEXIFOrientation(t *testing.T) {
	// Little-endian TIFF header with one Orientation SHORT entry = 6.
	payload := []byte{
		'E', 'x', 'i', 'f', 0, 0,
		'I', 'I', 42, 0, 8, 0, 0, 0,
		1, 0,
		0x12, 0x01, 3, 0, 1, 0, 0, 0, 6, 0, 0, 0,
		0, 0, 0, 0,
	}
	if got := parseEXIFOrientation(payload); got != 6 {
		t.Fatalf("orientation = %d, want 6", got)
	}
}

func TestWebPEncodeDecodeStatic(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 16, 12))
	for y := 0; y < 12; y++ {
		for x := 0; x < 16; x++ {
			src.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 10), G: uint8(y * 15), B: 90, A: 255})
		}
	}
	data, err := encodeWebP(src, 80)
	if err != nil {
		t.Fatal(err)
	}
	width, height, animated, err := webPFeatures(data)
	if err != nil {
		t.Fatal(err)
	}
	if animated || width != 16 || height != 12 {
		t.Fatalf("unexpected WebP features: %dx%d animated=%v", width, height, animated)
	}
}

func TestParseArgsV21Options(t *testing.T) {
	opts, err := parseArgs([]string{
		"photo.jpg", "--500", "--quality", "82", "--no-resize", "--output", "./optimized", "--dry-run", "--no-metadata",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.targetBytes != 500_000 || opts.quality != 82 || !opts.noResize || !opts.dryRun {
		t.Fatalf("unexpected options: %#v", opts)
	}
	if opts.outputDir != "./optimized" || opts.metadata != metadataNone {
		t.Fatalf("unexpected output/metadata settings: %#v", opts)
	}
}

func TestParseArgsConflictingMetadataOptions(t *testing.T) {
	if _, err := parseArgs([]string{"photo.jpg", "--500", "--keep-metadata", "--no-metadata"}); err == nil {
		t.Fatal("expected conflicting metadata options to fail")
	}
}

func TestWriteAtomicDoesNotOverwrite(t *testing.T) {
	path := t.TempDir() + "/existing.bin"
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(path, []byte("replacement")); err == nil {
		t.Fatal("expected writeAtomic to refuse an existing destination")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "original" {
		t.Fatalf("existing destination was modified: %q", got)
	}
}

func TestPNGMetadataInsertionPreservesReadablePNG(t *testing.T) {
	palette := color.Palette{color.NRGBA{R: 20, G: 40, B: 90, A: 255}, color.NRGBA{R: 220, G: 180, B: 40, A: 255}}
	img := image.NewPaletted(image.Rect(0, 0, 8, 8), palette)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.SetColorIndex(x, y, uint8((x+y)%2))
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	metadata := append(makePNGChunk("gAMA", []byte{0, 0, 0xb1, 0x8f}), makePNGChunk("pHYs", []byte{0, 0, 0x0e, 0xc4, 0, 0, 0x0e, 0xc4, 1})...)
	out, err := insertPNGMetadata(encoded.Bytes(), metadata)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := png.Decode(bytes.NewReader(out)); err != nil {
		t.Fatalf("metadata-inserted PNG does not decode: %v", err)
	}
	chunks, err := parsePNGChunks(out)
	if err != nil {
		t.Fatal(err)
	}
	positions := make(map[string]int)
	for i, chunk := range chunks {
		positions[chunk.typ] = i
	}
	if _, ok := positions["gAMA"]; !ok {
		t.Fatal("gAMA metadata chunk missing")
	}
	if _, ok := positions["pHYs"]; !ok {
		t.Fatal("pHYs metadata chunk missing")
	}
	if positions["gAMA"] > positions["PLTE"] || positions["pHYs"] > positions["IDAT"] {
		t.Fatalf("metadata chunks are in invalid order: %#v", positions)
	}
}

func TestJPEGMetadataInsertion(t *testing.T) {
	var encoded bytes.Buffer
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	if err := jpeg.Encode(&encoded, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	metadata := []byte{0xff, 0xe2, 0x00, 0x05, 't', 'e', 's'}
	out, err := insertJPEGMetadata(encoded.Bytes(), metadata)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != encoded.Len()+len(metadata) || !bytes.Equal(out[2:2+len(metadata)], metadata) {
		t.Fatal("JPEG metadata marker was not inserted after SOI")
	}
	if _, err := jpeg.Decode(bytes.NewReader(out)); err != nil {
		t.Fatalf("metadata-inserted JPEG does not decode: %v", err)
	}
}

func TestNormalizeEXIFOrientationMarker(t *testing.T) {
	payload := []byte{
		'E', 'x', 'i', 'f', 0, 0,
		'I', 'I', 42, 0, 8, 0, 0, 0,
		1, 0,
		0x12, 0x01, 3, 0, 1, 0, 0, 0, 6, 0, 0, 0,
		0, 0, 0, 0,
	}
	segmentLength := 2 + len(payload)
	segment := []byte{0xff, 0xe1, byte(segmentLength >> 8), byte(segmentLength)}
	segment = append(segment, payload...)
	normalizeEXIFOrientationMarker(segment)
	if got := parseEXIFOrientation(segment[4:]); got != 1 {
		t.Fatalf("normalized EXIF orientation = %d, want 1", got)
	}
}

func TestUniqueOutputPathAvoidsExistingFile(t *testing.T) {
	dir := t.TempDir()
	first := dir + "/photo_compressed.jpg"
	if err := os.WriteFile(first, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := uniqueOutputPath(dir+"/photo.jpg", "jpeg", dir)
	want := dir + "/photo_compressed_1.jpg"
	if got != want {
		t.Fatalf("uniqueOutputPath = %q, want %q", got, want)
	}
}

func TestExplicitTransformProcessesImageAlreadyUnderTarget(t *testing.T) {
	dir := t.TempDir()
	input := dir + "/small.png"
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	if err := os.WriteFile(input, encodeTestPNG(t, img), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(input)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() >= 100_000 {
		t.Fatalf("test source unexpectedly large: %d", info.Size())
	}
	stats, err := processFile(input, options{targetBytes: 100_000, quality: maxQuality, metadata: metadataNone})
	if err != nil {
		t.Fatal(err)
	}
	if stats.skipped || stats.outputPath == "" {
		t.Fatalf("explicit metadata transform was skipped: %#v", stats)
	}
	if _, err := os.Stat(stats.outputPath); err != nil {
		t.Fatalf("compressed copy was not written: %v", err)
	}
}

func encodeTestPNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestParseArgsV22Options(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sample.png"), encodeTestPNG(t, image.NewNRGBA(image.Rect(0, 0, 4, 4))), 0o644); err != nil {
		t.Fatal(err)
	}
	opts, err := parseArgs([]string{
		dir, "--500", "--recursive", "--format", "webp", "--json", "--output", "./optimized",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.recursive || !opts.jsonOutput || opts.outputFormat != "webp" || opts.outputDir != "./optimized" {
		t.Fatalf("unexpected v2.2 options: %#v", opts)
	}
	if len(opts.inputs) != 1 || filepath.Base(opts.inputs[0]) != "sample.png" {
		t.Fatalf("directory should be expanded during argument parsing, got %#v", opts.inputs)
	}
}

func TestNormalizeFormatAliases(t *testing.T) {
	got, err := normalizeFormat(".JPG")
	if err != nil {
		t.Fatal(err)
	}
	if got != "jpeg" {
		t.Fatalf("normalizeFormat = %q, want jpeg", got)
	}
	if _, err := normalizeFormat("avif"); err == nil {
		t.Fatal("expected unsupported AVIF format to be rejected")
	}
}

func TestExpandInputsRecursiveMirrorsAndSkipsOutput(t *testing.T) {
	base := t.TempDir()
	root := base + "/photos"
	output := root + "/optimized"
	if err := os.MkdirAll(root+"/trip/nested", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(output, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/trip/nested/a.png", encodeTestPNG(t, image.NewNRGBA(image.Rect(0, 0, 4, 4))), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(output+"/already.png", encodeTestPNG(t, image.NewNRGBA(image.Rect(0, 0, 2, 2))), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/trip/nested/a_compressed_1.webp", []byte("previous generated output"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/notes.txt", []byte("not an image"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, roots, err := expandInputs([]string{root}, true, output)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != root+"/trip/nested/a.png" {
		t.Fatalf("unexpected recursive files: %#v", files)
	}
	absRoot, _ := filepath.Abs(root)
	if roots[files[0]] != absRoot {
		t.Fatalf("source root = %q, want %q", roots[files[0]], absRoot)
	}
	gotOut := outputDirForInput(files[0], options{outputDir: output, sourceRoots: roots})
	if gotOut != output+"/trip/nested" {
		t.Fatalf("mirrored output dir = %q, want %q", gotOut, output+"/trip/nested")
	}
}

func TestRecursiveOutputCannotBeInputRoot(t *testing.T) {
	root := t.TempDir()
	if _, _, err := expandInputs([]string{root}, true, root); err == nil {
		t.Fatal("expected recursive output root to be rejected")
	}
}

func TestConvertPNGToWebP(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "source.png")
	img := image.NewNRGBA(image.Rect(0, 0, 80, 60))
	for y := 0; y < 60; y++ {
		for x := 0; x < 80; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 3), G: uint8(y * 4), B: 120, A: 255})
		}
	}
	if err := os.WriteFile(input, encodeTestPNG(t, img), 0o644); err != nil {
		t.Fatal(err)
	}
	stats, err := processFile(input, options{
		targetBytes: 200_000, quality: maxQuality, metadata: metadataPrivacy, outputFormat: "webp",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(stats.outputPath, "_compressed.webp") {
		t.Fatalf("unexpected converted output: %q", stats.outputPath)
	}
	if stats.outputBytes <= 0 || stats.outputBytes > 200_000 {
		t.Fatalf("converted output size = %d, want 1..200000", stats.outputBytes)
	}
	if _, animated, err := decodeWebPFile(stats.outputPath); err != nil || animated {
		t.Fatalf("converted WebP did not decode: animated=%v err=%v", animated, err)
	}
}

func TestAnimatedGIFConversionRequiresFirstFrame(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "animated.gif")
	palette := color.Palette{color.NRGBA{R: 255, A: 255}, color.NRGBA{G: 255, A: 255}}
	frame1 := image.NewPaletted(image.Rect(0, 0, 8, 8), palette)
	frame2 := image.NewPaletted(image.Rect(0, 0, 8, 8), palette)
	frame1.SetColorIndex(0, 0, 0)
	frame2.SetColorIndex(0, 0, 1)
	var data bytes.Buffer
	if err := gif.EncodeAll(&data, &gif.GIF{
		Image: []*image.Paletted{frame1, frame2}, Delay: []int{10, 10}, LoopCount: 0,
		Config: image.Config{ColorModel: palette, Width: 8, Height: 8},
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(input, data.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := processFile(input, options{targetBytes: 100_000, quality: maxQuality, metadata: metadataPrivacy, outputFormat: "jpeg"})
	if err == nil || !strings.Contains(err.Error(), "--first-frame") {
		t.Fatalf("expected first-frame requirement, got %v", err)
	}
	stats, err := processFile(input, options{
		targetBytes: 100_000, quality: maxQuality, metadata: metadataPrivacy, outputFormat: "jpeg", firstFrame: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(stats.outputPath, "_compressed.jpg") {
		t.Fatalf("unexpected first-frame output: %q", stats.outputPath)
	}
}

func TestJSONResultForSkippedFile(t *testing.T) {
	stats := fileStats{inputBytes: 1200, skipped: true, format: "jpeg"}
	got := makeJSONFileResult("photo.jpg", stats, "skipped", "")
	if got.OutputBytes != 1200 || got.SizeDelta != 0 || got.Status != "skipped" {
		t.Fatalf("unexpected JSON file result: %#v", got)
	}
}

func TestGeneratedOutputNameDetection(t *testing.T) {
	for _, path := range []string{"photo_compressed.jpg", "photo_compressed_1.webp"} {
		if !isScompressOutputName(path) {
			t.Errorf("expected generated output name to be detected: %s", path)
		}
	}
	for _, path := range []string{"photo.jpg", "photo_compressed_edit.jpg", "compressed_photo.jpg"} {
		if isScompressOutputName(path) {
			t.Errorf("unexpected generated output match: %s", path)
		}
	}
}

func TestConvertPNGToSingleFrameGIF(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "still.png")
	if err := os.WriteFile(input, encodeTestPNG(t, image.NewNRGBA(image.Rect(0, 0, 12, 10))), 0o644); err != nil {
		t.Fatal(err)
	}
	stats, err := processFile(input, options{
		targetBytes: 100_000, quality: maxQuality, metadata: metadataPrivacy, outputFormat: "gif",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(stats.outputPath, "_compressed.gif") {
		t.Fatalf("unexpected GIF output path: %q", stats.outputPath)
	}
	f, err := os.Open(stats.outputPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	decoded, err := gif.DecodeAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Image) != 1 {
		t.Fatalf("GIF frame count = %d, want 1", len(decoded.Image))
	}
}
