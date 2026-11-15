package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	version           = "2.4.2"
	minQuality        = 20
	maxQuality        = 95
	maxResizeAttempts = 32
	maxImagePixels    = int64(100_000_000)
)

type options struct {
	targetBytes  int64
	inputs       []string
	sourceRoots  map[string]string
	showVersion  bool
	dryRun       bool
	noResize     bool
	recursive    bool
	firstFrame   bool
	jsonOutput   bool
	quality      int
	maxWidth     int
	maxHeight    int
	outputDir    string
	outputFormat string
	metadata     metadataMode
}

type metadataMode int

const (
	metadataPrivacy metadataMode = iota // keep color-management metadata, remove identifying metadata
	metadataNone
	metadataKeep
)

type source struct {
	format string
	image  image.Image
	gif    *gif.GIF
}

type result struct {
	data   []byte
	width  int
	height int
	method string
}

type fileStats struct {
	inputBytes  int64
	outputBytes int64
	outputPath  string
	skipped     bool
	dryRun      bool
	format      string
	width       int
	height      int
	method      string
}

type batchStats struct {
	files       int
	compressed  int
	skipped     int
	dryRun      int
	failed      int
	inputBytes  int64
	outputBytes int64
}

type jsonFileResult struct {
	Input       string  `json:"input"`
	Status      string  `json:"status"`
	Output      string  `json:"output,omitempty"`
	Format      string  `json:"format,omitempty"`
	InputBytes  int64   `json:"input_bytes"`
	OutputBytes int64   `json:"output_bytes"`
	SizeDelta   int64   `json:"size_delta_bytes"`
	SizeChange  float64 `json:"size_change_percent"`
	Width       int     `json:"width,omitempty"`
	Height      int     `json:"height,omitempty"`
	Method      string  `json:"method,omitempty"`
	Error       string  `json:"error,omitempty"`
}

type jsonSummary struct {
	Total       int     `json:"total"`
	Compressed  int     `json:"compressed"`
	Skipped     int     `json:"skipped"`
	DryRun      int     `json:"dry_run"`
	Failed      int     `json:"failed"`
	InputBytes  int64   `json:"input_bytes"`
	OutputBytes int64   `json:"output_bytes"`
	SizeDelta   int64   `json:"size_delta_bytes"`
	SizeChange  float64 `json:"size_change_percent"`
}

type jsonReport struct {
	Version string           `json:"version"`
	Files   []jsonFileResult `json:"files"`
	Summary jsonSummary      `json:"summary"`
}

func main() {
	opts, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "scompress:", err)
		printUsage(os.Stderr)
		os.Exit(2)
	}
	if opts.showVersion {
		fmt.Printf("scompress %s\n", version)
		return
	}

	var summary batchStats
	report := jsonReport{Version: version, Files: make([]jsonFileResult, 0, len(opts.inputs))}
	for _, input := range opts.inputs {
		summary.files++
		stats, err := processFile(input, opts)
		summary.inputBytes += stats.inputBytes
		if err != nil {
			if !opts.jsonOutput {
				fmt.Fprintf(os.Stderr, "✗ %s: %v\n", input, err)
			}
			summary.failed++
			summary.outputBytes += stats.inputBytes // A failed file remains unchanged.
			report.Files = append(report.Files, makeJSONFileResult(input, stats, "failed", err.Error()))
			continue
		}
		if stats.skipped {
			summary.skipped++
			summary.outputBytes += stats.inputBytes
			report.Files = append(report.Files, makeJSONFileResult(input, stats, "skipped", ""))
			continue
		}
		summary.outputBytes += stats.outputBytes
		status := "compressed"
		if stats.dryRun {
			status = "dry_run"
			summary.dryRun++
		} else {
			summary.compressed++
		}
		report.Files = append(report.Files, makeJSONFileResult(input, stats, status, ""))
	}
	if opts.jsonOutput {
		report.Summary = jsonSummary{
			Total: summary.files, Compressed: summary.compressed, Skipped: summary.skipped,
			DryRun: summary.dryRun, Failed: summary.failed, InputBytes: summary.inputBytes, OutputBytes: summary.outputBytes,
			SizeDelta:  summary.inputBytes - summary.outputBytes,
			SizeChange: percentSaved(summary.inputBytes, summary.outputBytes),
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintln(os.Stderr, "scompress: encode JSON report:", err)
			os.Exit(2)
		}
	} else if summary.files > 1 {
		printBatchSummary(summary)
	}
	if summary.failed > 0 {
		os.Exit(1)
	}
}

func makeJSONFileResult(input string, stats fileStats, status, message string) jsonFileResult {
	outputBytes := stats.outputBytes
	if status == "skipped" {
		outputBytes = stats.inputBytes
	}
	if status == "failed" {
		outputBytes = stats.inputBytes // Failed files remain unchanged in the aggregate.
	}
	delta := stats.inputBytes - outputBytes
	return jsonFileResult{
		Input: input, Status: status, Output: stats.outputPath, Format: stats.format,
		InputBytes: stats.inputBytes, OutputBytes: outputBytes, SizeDelta: delta,
		SizeChange: percentSaved(stats.inputBytes, outputBytes),
		Width:      stats.width, Height: stats.height, Method: stats.method, Error: message,
	}
}

func parseArgs(args []string) (options, error) {
	out := options{quality: maxQuality, metadata: metadataPrivacy}
	var targetKB int64
	metadataFlagSeen := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help":
			printUsage(os.Stdout)
			os.Exit(0)
		case arg == "-v" || arg == "--version":
			out.showVersion = true
		case arg == "--dry-run":
			out.dryRun = true
		case arg == "--json":
			out.jsonOutput = true
		case arg == "--recursive" || arg == "-r":
			out.recursive = true
		case arg == "--first-frame":
			out.firstFrame = true
		case arg == "--no-resize":
			out.noResize = true
		case arg == "--no-metadata":
			if metadataFlagSeen {
				return out, errors.New("choose only one of --no-metadata and --keep-metadata")
			}
			metadataFlagSeen = true
			out.metadata = metadataNone
		case arg == "--keep-metadata":
			if metadataFlagSeen {
				return out, errors.New("choose only one of --no-metadata and --keep-metadata")
			}
			metadataFlagSeen = true
			out.metadata = metadataKeep
		case strings.HasPrefix(arg, "--target="):
			v := strings.TrimPrefix(arg, "--target=")
			n, err := positiveInt64(v)
			if err != nil {
				return out, fmt.Errorf("invalid target size %q (use a positive KB value)", v)
			}
			targetKB = n
		case strings.HasPrefix(arg, "--quality="):
			v := strings.TrimPrefix(arg, "--quality=")
			n, err := parseQuality(v)
			if err != nil {
				return out, err
			}
			out.quality = n
		case strings.HasPrefix(arg, "--max-width="):
			v := strings.TrimPrefix(arg, "--max-width=")
			n, err := positiveInt(v)
			if err != nil {
				return out, fmt.Errorf("invalid maximum width %q", v)
			}
			out.maxWidth = n
		case strings.HasPrefix(arg, "--max-height="):
			v := strings.TrimPrefix(arg, "--max-height=")
			n, err := positiveInt(v)
			if err != nil {
				return out, fmt.Errorf("invalid maximum height %q", v)
			}
			out.maxHeight = n
		case strings.HasPrefix(arg, "--output="):
			out.outputDir = strings.TrimPrefix(arg, "--output=")
			if out.outputDir == "" {
				return out, errors.New("--output requires a directory")
			}
		case strings.HasPrefix(arg, "--format="):
			v := strings.TrimPrefix(arg, "--format=")
			format, err := normalizeFormat(v)
			if err != nil {
				return out, err
			}
			out.outputFormat = format
		case arg == "--target" || arg == "-t":
			v, err := nextArg(args, &i, arg)
			if err != nil {
				return out, err
			}
			n, parseErr := positiveInt64(v)
			if parseErr != nil {
				return out, fmt.Errorf("invalid target size %q (use a positive KB value)", v)
			}
			targetKB = n
		case arg == "--quality":
			v, err := nextArg(args, &i, arg)
			if err != nil {
				return out, err
			}
			n, parseErr := parseQuality(v)
			if parseErr != nil {
				return out, parseErr
			}
			out.quality = n
		case arg == "--max-width":
			v, err := nextArg(args, &i, arg)
			if err != nil {
				return out, err
			}
			n, parseErr := positiveInt(v)
			if parseErr != nil {
				return out, fmt.Errorf("invalid maximum width %q", v)
			}
			out.maxWidth = n
		case arg == "--max-height":
			v, err := nextArg(args, &i, arg)
			if err != nil {
				return out, err
			}
			n, parseErr := positiveInt(v)
			if parseErr != nil {
				return out, fmt.Errorf("invalid maximum height %q", v)
			}
			out.maxHeight = n
		case arg == "--output" || arg == "-o":
			v, err := nextArg(args, &i, arg)
			if err != nil {
				return out, err
			}
			out.outputDir = v
		case arg == "--format" || arg == "-f":
			v, err := nextArg(args, &i, arg)
			if err != nil {
				return out, err
			}
			format, parseErr := normalizeFormat(v)
			if parseErr != nil {
				return out, parseErr
			}
			out.outputFormat = format
		case strings.HasPrefix(arg, "--") && len(arg) > 2 && allDigits(arg[2:]):
			n, err := positiveInt64(arg[2:])
			if err != nil {
				return out, fmt.Errorf("invalid target size %q", arg)
			}
			targetKB = n
		case strings.HasPrefix(arg, "-"):
			return out, fmt.Errorf("unknown option %q", arg)
		default:
			out.inputs = append(out.inputs, arg)
		}
	}

	if out.showVersion && len(out.inputs) == 0 {
		return out, nil
	}
	if targetKB == 0 {
		return out, errors.New("missing target size; use syntax like --500 or --target 500")
	}
	if len(out.inputs) == 0 {
		return out, errors.New("no input image specified")
	}
	if out.noResize && (out.maxWidth > 0 || out.maxHeight > 0) {
		return out, errors.New("--no-resize cannot be combined with --max-width or --max-height")
	}
	if out.firstFrame && out.outputFormat == "" {
		return out, errors.New("--first-frame is only useful with --format to a still-image format")
	}
	if out.firstFrame && out.outputFormat == "gif" {
		return out, errors.New("--first-frame cannot be combined with --format gif")
	}
	if targetKB > math.MaxInt64/1000 {
		return out, errors.New("target size is too large")
	}
	out.targetBytes = targetKB * 1000 // Decimal KB keeps output below strict upload limits.
	var err error
	out.inputs, out.sourceRoots, err = expandInputs(out.inputs, out.recursive, out.outputDir)
	if err != nil {
		return out, err
	}
	if len(out.inputs) == 0 {
		return out, errors.New("no matching input files found")
	}
	return out, nil
}

func normalizeFormat(value string) (string, error) {
	format := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(value), "."))
	if format == "jpg" {
		format = "jpeg"
	}
	switch format {
	case "jpeg", "png", "gif", "webp":
		return format, nil
	default:
		return "", fmt.Errorf("unsupported output format %q (choose jpeg, png, gif, or webp)", value)
	}
}

func nextArg(args []string, index *int, flag string) (string, error) {
	if *index+1 >= len(args) {
		return "", fmt.Errorf("%s requires a value", flag)
	}
	*index = *index + 1
	return args[*index], nil
}

func positiveInt64(s string) (int64, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 1 {
		return 0, errors.New("not a positive integer")
	}
	return n, nil
}

func positiveInt(s string) (int, error) {
	n, err := positiveInt64(s)
	if err != nil || n > int64(math.MaxInt) {
		return 0, errors.New("not a positive integer")
	}
	return int(n), nil
}

func parseQuality(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n < minQuality || n > maxQuality {
		return 0, fmt.Errorf("quality must be between %d and %d", minQuality, maxQuality)
	}
	return n, nil
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func expandInputs(inputs []string, recursive bool, outputDir string) ([]string, map[string]string, error) {
	var expanded []string
	roots := make(map[string]string)
	seen := make(map[string]bool)
	for _, input := range inputs {
		matches, err := filepath.Glob(input)
		if err != nil {
			return nil, nil, err
		}
		if len(matches) == 0 {
			matches = []string{input}
		}
		for _, match := range matches {
			clean := filepath.Clean(match)
			info, statErr := os.Stat(clean)
			if statErr == nil && info.IsDir() {
				if !recursive {
					return nil, nil, fmt.Errorf("%q is a directory; use --recursive to process its images", match)
				}
				root, absErr := filepath.Abs(clean)
				if absErr != nil {
					return nil, nil, absErr
				}
				outputAbs := ""
				if outputDir != "" {
					outputAbs, absErr = filepath.Abs(outputDir)
					if absErr != nil {
						return nil, nil, absErr
					}
					if filepath.Clean(outputAbs) == filepath.Clean(root) {
						return nil, nil, errors.New("--output cannot be the recursive input root; choose a separate output subdirectory")
					}
				}
				walkErr := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
					if walkErr != nil {
						return walkErr
					}
					if entry.IsDir() {
						if outputAbs != "" && filepath.Clean(path) == filepath.Clean(outputAbs) {
							return filepath.SkipDir
						}
						return nil
					}
					if !entry.Type().IsRegular() || !isSupportedImageExtension(path) || isScompressOutputName(path) {
						return nil
					}
					file := filepath.Clean(path)
					if !seen[file] {
						seen[file] = true
						expanded = append(expanded, file)
						roots[file] = root
					}
					return nil
				})
				if walkErr != nil {
					return nil, nil, fmt.Errorf("walk directory %q: %w", match, walkErr)
				}
				continue
			}
			if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
				return nil, nil, statErr
			}
			if !seen[clean] {
				seen[clean] = true
				expanded = append(expanded, clean)
			}
		}
	}
	sort.Strings(expanded)
	return expanded, roots, nil
}

func isScompressOutputName(path string) bool {
	stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	idx := strings.LastIndex(stem, "_compressed")
	if idx < 0 {
		return false
	}
	suffix := stem[idx+len("_compressed"):]
	if suffix == "" {
		return true
	}
	return strings.HasPrefix(suffix, "_") && allDigits(strings.TrimPrefix(suffix, "_"))
}

func isSupportedImageExtension(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		return true
	default:
		return false
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `scompress — standalone image compressor

Usage:
  scompress image.png --500
  scompress photo.jpg --target 300
  scompress *.jpg --500 --output ./optimized
  scompress photo.png --500 --format webp
  scompress ./photos --500 --recursive --output ./optimized --json

The size is in decimal KB (1 KB = 1,000 bytes). Originals are never changed.
Supported formats: JPEG, PNG, GIF (including animation), and static WebP.

Options:
  --500                 Maximum output size in KB (short syntax)
  --target 500          Same as --500
  --quality 85          Preferred maximum quality (20–95); lower if needed
  --no-resize           Never reduce image dimensions
  --max-width 1920      Limit output width (never upscales)
  --max-height 1080     Limit output height (never upscales)
  --output DIR, -o DIR  Save outputs in DIR instead of beside originals
  --format FORMAT, -f   Convert output to jpeg, png, gif, or webp
  --recursive, -r       Process supported images in directories recursively
  --first-frame         Allow animated GIF input to become a still output image
  --json                Print machine-readable JSON instead of human output
  --dry-run             Compress in memory but do not write output files
  --no-metadata         Remove metadata and color-profile chunks
  --keep-metadata       Keep supported metadata, including EXIF/GPS
  -h, --help            Show this help
  -v, --version         Show version

Metadata policy by default: identifying metadata is removed; supported color
profiles and PNG physical-size information are retained. --keep-metadata is
currently supported for JPEG and PNG. GIF animation timing and loop count are
preserved; GIF comments and application extensions are not retained.`)
}

func processFile(path string, opts options) (fileStats, error) {
	var stats fileStats
	stats.format = strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if stats.format == "jpg" {
		stats.format = "jpeg"
	}
	info, err := os.Stat(path)
	if err != nil {
		return stats, err
	}
	stats.inputBytes = info.Size()
	if !info.Mode().IsRegular() {
		return stats, errors.New("not a regular file")
	}
	needsExplicitTransform := opts.maxWidth > 0 || opts.maxHeight > 0 || opts.quality < maxQuality || opts.metadata != metadataPrivacy || opts.outputFormat != ""
	if info.Size() <= opts.targetBytes && !needsExplicitTransform {
		stats.skipped = true
		if !opts.jsonOutput {
			fmt.Printf("• %s: already under target (%s)\n", filepath.Base(path), humanSize(info.Size()))
		}
		return stats, nil
	}

	src, err := loadImage(path)
	if err != nil {
		return stats, err
	}
	outputFormat := src.format
	if opts.outputFormat != "" {
		outputFormat = opts.outputFormat
	}
	stats.format = outputFormat

	if src.gif != nil && outputFormat != "gif" {
		if len(src.gif.Image) > 1 && !opts.firstFrame {
			return stats, errors.New("animated GIF cannot be converted to a still format without --first-frame")
		}
		src.image = firstGIFFrame(src.gif)
	}

	var metadata []byte
	if outputFormat != src.format && opts.metadata == metadataKeep {
		return stats, errors.New("--keep-metadata is not supported during cross-format conversion; use the default privacy mode")
	}
	if outputFormat == src.format {
		metadata, err = imageMetadata(path, src.format, opts.metadata)
		if err != nil {
			return stats, err
		}
	}
	encodeTarget := opts.targetBytes - int64(len(metadata))
	if encodeTarget <= 0 {
		return stats, errors.New("preserved metadata alone exceeds the requested target size")
	}

	var res result
	switch outputFormat {
	case "jpeg":
		res, err = compressJPEG(flattenForJPEG(src.image), encodeTarget, opts)
	case "png":
		res, err = compressPNG(src.image, encodeTarget, opts)
	case "gif":
		gifSource := src.gif
		if gifSource == nil {
			gifSource = singleFrameGIF(src.image)
		}
		res, err = compressGIF(gifSource, encodeTarget, opts)
	case "webp":
		res, err = compressWebP(src.image, encodeTarget, opts)
	default:
		err = fmt.Errorf("unsupported image format %q", src.format)
	}
	if err != nil {
		return stats, err
	}
	if len(metadata) > 0 {
		switch outputFormat {
		case "jpeg":
			res.data, err = insertJPEGMetadata(res.data, metadata)
		case "png":
			res.data, err = insertPNGMetadata(res.data, metadata)
		}
		if err != nil {
			return stats, err
		}
	}
	if int64(len(res.data)) > opts.targetBytes {
		return stats, fmt.Errorf("could not reach target size (%s) after metadata; no output was written", humanSize(opts.targetBytes))
	}

	outDir := outputDirForInput(path, opts)
	outPath := uniqueOutputPath(path, outputFormat, outDir)
	stats.outputBytes = int64(len(res.data))
	stats.outputPath = outPath
	stats.dryRun = opts.dryRun
	stats.width, stats.height, stats.method = res.width, res.height, res.method
	if opts.dryRun {
		if !opts.jsonOutput {
			fmt.Printf("◦ %s: %s → %s (%s; dry run)\n", filepath.Base(path), humanSize(info.Size()), humanSize(stats.outputBytes), sizeChangeText(info.Size(), stats.outputBytes))
			fmt.Printf("  Would save: %s\n", outPath)
			fmt.Printf("  Method: %s; output dimensions %d×%d\n", res.method, res.width, res.height)
		}
		return stats, nil
	}
	if outDir != "" {
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			return stats, fmt.Errorf("create output directory: %w", err)
		}
		// Re-check after creating the directory, and keep writeAtomic's no-overwrite guarantee.
		outPath = uniqueOutputPath(path, outputFormat, outDir)
		stats.outputPath = outPath
	}
	if err := writeAtomic(outPath, res.data); err != nil {
		return stats, err
	}

	if !opts.jsonOutput {
		fmt.Printf("✓ %s: %s → %s (%s)\n", filepath.Base(path), humanSize(info.Size()), humanSize(stats.outputBytes), sizeChangeText(info.Size(), stats.outputBytes))
		fmt.Printf("  Saved: %s\n", outPath)
		fmt.Printf("  Method: %s; output dimensions %d×%d\n", res.method, res.width, res.height)
	}
	return stats, nil
}

func sizeChangeText(original, output int64) string {
	if original <= 0 || output == original {
		return "same size"
	}
	if output < original {
		return fmt.Sprintf("%.1f%% smaller", percentSaved(original, output))
	}
	return fmt.Sprintf("%.1f%% larger", 100*(float64(output)/float64(original)-1))
}

func outputDirForInput(path string, opts options) string {
	if opts.outputDir == "" {
		return ""
	}
	if root, ok := opts.sourceRoots[path]; ok {
		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err == nil && rel != "." {
			return filepath.Join(opts.outputDir, rel)
		}
	}
	return opts.outputDir
}

func firstGIFFrame(src *gif.GIF) image.Image {
	if src == nil || len(src.Image) == 0 {
		return nil
	}
	w, h := src.Config.Width, src.Config.Height
	if w <= 0 || h <= 0 {
		b := src.Image[0].Bounds()
		w, h = b.Max.X, b.Max.Y
	}
	canvas := image.NewNRGBA(image.Rect(0, 0, w, h))
	frame := src.Image[0]
	draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)
	return canvas
}

func singleFrameGIF(img image.Image) *gif.GIF {
	if img == nil {
		return nil
	}
	b := img.Bounds()
	paletted := toPaletted(img, 256)
	return &gif.GIF{
		Image: []*image.Paletted{paletted}, Delay: []int{0}, LoopCount: -1,
		Disposal: []byte{gif.DisposalNone},
		Config:   image.Config{ColorModel: paletted.Palette, Width: b.Dx(), Height: b.Dy()},
	}
}

func flattenForJPEG(img image.Image) image.Image {
	if img == nil {
		return nil
	}
	b := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(out, out.Bounds(), img, b.Min, draw.Over)
	return out
}

func loadImage(path string) (*source, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".webp" {
		img, animated, err := decodeWebPFile(path)
		if err != nil {
			return nil, fmt.Errorf("cannot decode WebP: %w", err)
		}
		if animated {
			return nil, errors.New("animated WebP is not supported yet; this file was left unchanged")
		}
		if img == nil {
			return nil, errors.New("WebP decoder returned no image")
		}
		if int64(img.Bounds().Dx())*int64(img.Bounds().Dy()) > maxImagePixels {
			return nil, errors.New("image exceeds the 100-megapixel safety limit")
		}
		return &source{format: "webp", image: img}, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	config, format, err := image.DecodeConfig(f)
	if err != nil {
		return nil, fmt.Errorf("cannot read image header: %w", err)
	}
	if config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > maxImagePixels {
		return nil, errors.New("image dimensions are invalid or exceed the 100-megapixel safety limit")
	}
	if format != "jpeg" && format != "png" && format != "gif" {
		return nil, fmt.Errorf("unsupported format %q (supported: JPEG, PNG, GIF, WebP)", format)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	if format == "gif" {
		g, err := gif.DecodeAll(f)
		if err != nil {
			return nil, fmt.Errorf("cannot decode GIF: %w", err)
		}
		if len(g.Image) == 0 {
			return nil, errors.New("GIF contains no frames")
		}
		return &source{format: "gif", gif: g}, nil
	}
	img, decodedFormat, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("cannot decode image: %w", err)
	}
	if decodedFormat == "jpeg" {
		if orientation := readJPEGOrientation(path); orientation >= 2 && orientation <= 8 {
			img = applyOrientation(img, orientation)
		}
	}
	return &source{format: decodedFormat, image: img}, nil
}

func compressJPEG(img image.Image, target int64, opts options) (result, error) {
	return compressWithQuality(img, target, opts, func(candidate image.Image, quality int) ([]byte, error) {
		var buf bytes.Buffer
		err := encodeJPEG(&buf, candidate, quality)
		return buf.Bytes(), err
	}, "JPEG")
}

func compressWebP(img image.Image, target int64, opts options) (result, error) {
	return compressWithQuality(img, target, opts, func(candidate image.Image, quality int) ([]byte, error) {
		return encodeWebP(candidate, quality)
	}, "WebP")
}

func compressWithQuality(img image.Image, target int64, opts options, encode func(image.Image, int) ([]byte, error), label string) (result, error) {
	baseScale := dimensionScale(img.Bounds().Dx(), img.Bounds().Dy(), opts.maxWidth, opts.maxHeight)
	if opts.noResize {
		baseScale = 1
	}
	for attempt, relativeScale := 0, 1.0; attempt < maxResizeAttempts; attempt, relativeScale = attempt+1, relativeScale*0.92 {
		if opts.noResize && attempt > 0 {
			break
		}
		scale := baseScale * relativeScale
		candidate := img
		if scale < 0.999999 {
			candidate = resize(img, scale)
		}
		data, quality, ok := bestQualityLimit(target, opts.quality, func(q int) ([]byte, error) {
			return encode(candidate, q)
		})
		if ok {
			b := candidate.Bounds()
			return result{data: data, width: b.Dx(), height: b.Dy(), method: fmt.Sprintf("%s quality %d", label, quality)}, nil
		}
		if opts.noResize {
			return result{}, fmt.Errorf("cannot reach %s without resizing; try a larger target or allow resizing", humanSize(target))
		}
	}
	return result{}, fmt.Errorf("could not reach %s without reducing the image below a practical size", humanSize(target))
}

func bestQualityLimit(target int64, maxQ int, encode func(int) ([]byte, error)) ([]byte, int, bool) {
	var best []byte
	bestQ := 0
	low, high := minQuality, maxQ
	for low <= high {
		q := (low + high) / 2
		data, err := encode(q)
		if err != nil {
			return nil, 0, false
		}
		if int64(len(data)) <= target {
			best, bestQ = data, q
			low = q + 1
		} else {
			high = q - 1
		}
	}
	return best, bestQ, len(best) > 0
}

func encodeJPEG(w io.Writer, img image.Image, quality int) error {
	return jpeg.Encode(w, img, &jpeg.Options{Quality: quality})
}

func compressPNG(img image.Image, target int64, opts options) (result, error) {
	baseScale := dimensionScale(img.Bounds().Dx(), img.Bounds().Dy(), opts.maxWidth, opts.maxHeight)
	if opts.noResize {
		baseScale = 1
	}
	paletteLimit := maxInt(2, int(math.Round(256*float64(opts.quality)/float64(maxQuality))))
	highPaletteSteps := pngHighPaletteCandidates(paletteLimit)
	lowPaletteSteps := []int{16, 8, 4, 2}

	currentScale := baseScale
	for attempt := 0; attempt < maxResizeAttempts; attempt++ {
		candidate := img
		if currentScale < 0.999999 {
			candidate = resize(img, currentScale)
		}
		var lossless bytes.Buffer
		pngEncoder := png.Encoder{CompressionLevel: png.BestCompression}
		if err := pngEncoder.Encode(&lossless, candidate); err != nil {
			return result{}, err
		}
		if int64(lossless.Len()) <= target {
			b := candidate.Bounds()
			return result{data: lossless.Bytes(), width: b.Dx(), height: b.Dy(), method: "PNG lossless"}, nil
		}

		coarsestSize := 0
		for _, colors := range highPaletteSteps {
			paletted := toPalettedRGBA(candidate, colors)
			var buf bytes.Buffer
			enc := png.Encoder{CompressionLevel: png.BestCompression}
			if err := enc.Encode(&buf, paletted); err != nil {
				return result{}, err
			}
			if int64(buf.Len()) <= target {
				b := candidate.Bounds()
				return result{data: buf.Bytes(), width: b.Dx(), height: b.Dy(), method: fmt.Sprintf("PNG adaptive palette (%d colors)", len(paletted.Palette))}, nil
			}
			coarsestSize = buf.Len()
		}

		if opts.noResize {
			for _, colors := range lowPaletteSteps {
				paletted := toPalettedRGBA(candidate, colors)
				var buf bytes.Buffer
				enc := png.Encoder{CompressionLevel: png.BestCompression}
				if err := enc.Encode(&buf, paletted); err != nil {
					return result{}, err
				}
				if int64(buf.Len()) <= target {
					b := candidate.Bounds()
					return result{data: buf.Bytes(), width: b.Dx(), height: b.Dy(), method: fmt.Sprintf("PNG adaptive palette (%d colors)", len(paletted.Palette))}, nil
				}
			}
			return result{}, fmt.Errorf("cannot reach %s without resizing; try a larger target or allow resizing", humanSize(target))
		}

		if coarsestSize <= 0 {
			return result{}, errors.New("PNG encoder produced no candidate output")
		}
		// Estimate the next scale from the coarsest acceptable palette tier.
		// This avoids aggressively reducing colors on a full-resolution photo
		// when a modest dimension reduction can retain more color detail.
		factor := 0.90 * math.Sqrt(float64(target)/float64(coarsestSize))
		if factor > 0.90 {
			factor = 0.90
		}
		if factor < 0.45 {
			factor = 0.45
		}
		currentScale *= factor
		if currentScale < 0.04 {
			break
		}
	}

	// Last-resort palette reduction is used only after dimensional compression
	// has reached its practical floor.
	candidate := resize(img, currentScale)
	for _, colors := range lowPaletteSteps {
		paletted := toPalettedRGBA(candidate, colors)
		var buf bytes.Buffer
		enc := png.Encoder{CompressionLevel: png.BestCompression}
		if err := enc.Encode(&buf, paletted); err != nil {
			return result{}, err
		}
		if int64(buf.Len()) <= target {
			b := candidate.Bounds()
			return result{data: buf.Bytes(), width: b.Dx(), height: b.Dy(), method: fmt.Sprintf("PNG adaptive palette (%d colors)", len(paletted.Palette))}, nil
		}
	}
	return result{}, fmt.Errorf("could not reach %s; try a larger target size", humanSize(target))
}

func pngHighPaletteCandidates(limit int) []int {
	out := []int{limit}
	for _, n := range []int{128, 64, 32} {
		if n < limit {
			out = append(out, n)
		}
	}
	return out
}

func compressGIF(src *gif.GIF, target int64, opts options) (result, error) {
	if src == nil || len(src.Image) == 0 {
		return result{}, errors.New("GIF contains no frames")
	}
	currentScale := dimensionScale(src.Config.Width, src.Config.Height, opts.maxWidth, opts.maxHeight)
	if opts.noResize {
		currentScale = 1
	}
	paletteLimit := maxInt(2, int(math.Round(256*float64(opts.quality)/float64(maxQuality))))

	for attempt := 0; attempt < maxResizeAttempts; attempt++ {
		resizedFrames := make([]image.Image, len(src.Image))
		for i, frame := range src.Image {
			resizedFrames[i] = frame
			if currentScale < 0.999999 {
				resizedFrames[i] = resizeWithOrigin(frame, currentScale)
			}
		}
		frames := toPalettedFrames(resizedFrames, paletteLimit)
		cfgW := maxInt(1, int(math.Round(float64(src.Config.Width)*currentScale)))
		cfgH := maxInt(1, int(math.Round(float64(src.Config.Height)*currentScale)))
		backgroundIndex := byte(0)
		if originalPalette, ok := src.Config.ColorModel.(color.Palette); ok && int(src.BackgroundIndex) < len(originalPalette) {
			backgroundIndex = byte(frames[0].Palette.Index(originalPalette[src.BackgroundIndex]))
		}
		g := &gif.GIF{
			Image:           frames,
			Delay:           append([]int(nil), src.Delay...),
			LoopCount:       src.LoopCount,
			Disposal:        append([]byte(nil), src.Disposal...),
			BackgroundIndex: backgroundIndex,
			Config:          image.Config{ColorModel: frames[0].Palette, Width: cfgW, Height: cfgH},
		}
		var buf bytes.Buffer
		if err := gif.EncodeAll(&buf, g); err != nil {
			return result{}, err
		}
		if int64(buf.Len()) <= target {
			return result{data: buf.Bytes(), width: cfgW, height: cfgH, method: fmt.Sprintf("GIF shared palette (%d colors), %d frames", len(frames[0].Palette), len(frames))}, nil
		}
		if opts.noResize {
			return result{}, fmt.Errorf("cannot reach %s without resizing; try a larger target or allow resizing", humanSize(target))
		}

		// Pixel count is approximately proportional to encoded GIF size. Use
		// that estimate to jump toward a useful scale, then refine on the next
		// pass. The palette is held constant so dimensions are reduced before
		// color fidelity is sacrificed.
		factor := 0.90 * math.Sqrt(float64(target)/float64(buf.Len()))
		if factor > 0.90 {
			factor = 0.90
		}
		if factor < 0.45 {
			factor = 0.45
		}
		currentScale *= factor
		if currentScale < 0.04 {
			break
		}
	}
	return result{}, fmt.Errorf("could not reach %s without reducing the GIF below a practical size", humanSize(target))
}

func dimensionScale(width, height, maxWidth, maxHeight int) float64 {
	scale := 1.0
	if maxWidth > 0 && width > maxWidth {
		scale = math.Min(scale, float64(maxWidth)/float64(width))
	}
	if maxHeight > 0 && height > maxHeight {
		scale = math.Min(scale, float64(maxHeight)/float64(height))
	}
	return scale
}

func resize(src image.Image, scale float64) *image.NRGBA {
	b := src.Bounds()
	w := maxInt(1, int(math.Round(float64(b.Dx())*scale)))
	h := maxInt(1, int(math.Round(float64(b.Dy())*scale)))
	return resizeToRect(src, image.Rect(0, 0, w, h))
}

func resizeWithOrigin(src image.Image, scale float64) *image.NRGBA {
	b := src.Bounds()
	minX := int(math.Round(float64(b.Min.X) * scale))
	minY := int(math.Round(float64(b.Min.Y) * scale))
	maxX := maxInt(minX+1, int(math.Round(float64(b.Max.X)*scale)))
	maxY := maxInt(minY+1, int(math.Round(float64(b.Max.Y)*scale)))
	return resizeToRect(src, image.Rect(minX, minY, maxX, maxY))
}

// resizeToRect uses bilinear interpolation and has no external image-library dependency.
func resizeToRect(src image.Image, dstRect image.Rectangle) *image.NRGBA {
	sb := src.Bounds()
	dst := image.NewNRGBA(dstRect)
	sw, sh := sb.Dx(), sb.Dy()
	dw, dh := dstRect.Dx(), dstRect.Dy()
	if sw <= 0 || sh <= 0 || dw <= 0 || dh <= 0 {
		return dst
	}
	for y := 0; y < dh; y++ {
		sy := (float64(y)+0.5)*float64(sh)/float64(dh) - 0.5
		y0 := int(math.Floor(sy))
		fy := sy - float64(y0)
		if y0 < 0 {
			y0, fy = 0, 0
		}
		y1 := minInt(y0+1, sh-1)
		for x := 0; x < dw; x++ {
			sx := (float64(x)+0.5)*float64(sw)/float64(dw) - 0.5
			x0 := int(math.Floor(sx))
			fx := sx - float64(x0)
			if x0 < 0 {
				x0, fx = 0, 0
			}
			x1 := minInt(x0+1, sw-1)
			c00 := color.NRGBAModel.Convert(src.At(sb.Min.X+x0, sb.Min.Y+y0)).(color.NRGBA)
			c10 := color.NRGBAModel.Convert(src.At(sb.Min.X+x1, sb.Min.Y+y0)).(color.NRGBA)
			c01 := color.NRGBAModel.Convert(src.At(sb.Min.X+x0, sb.Min.Y+y1)).(color.NRGBA)
			c11 := color.NRGBAModel.Convert(src.At(sb.Min.X+x1, sb.Min.Y+y1)).(color.NRGBA)
			lerp := func(a, b, c, d uint8) uint8 {
				top := float64(a)*(1-fx) + float64(b)*fx
				bot := float64(c)*(1-fx) + float64(d)*fx
				v := top*(1-fy) + bot*fy
				if v < 0 {
					v = 0
				}
				if v > 255 {
					v = 255
				}
				return uint8(math.Round(v))
			}
			dst.SetNRGBA(dstRect.Min.X+x, dstRect.Min.Y+y, color.NRGBA{
				R: lerp(c00.R, c10.R, c01.R, c11.R),
				G: lerp(c00.G, c10.G, c01.G, c11.G),
				B: lerp(c00.B, c10.B, c01.B, c11.B),
				A: lerp(c00.A, c10.A, c01.A, c11.A),
			})
		}
	}
	return dst
}

func uniqueOutputPath(input, format, outputDir string) string {
	dir := outputDir
	if dir == "" {
		dir = filepath.Dir(input)
	}
	base := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
	ext := "." + format
	if format == "jpeg" {
		ext = ".jpg"
	}
	candidate := filepath.Join(dir, base+"_compressed"+ext)
	for i := 1; ; i++ {
		if _, err := os.Lstat(candidate); errors.Is(err, os.ErrNotExist) || err != nil {
			// If the directory is inaccessible, return the candidate and let the
			// writer report the permission error. Never spin on repeated EACCES.
			return candidate
		}
		candidate = filepath.Join(dir, fmt.Sprintf("%s_compressed_%d%s", base, i, ext))
	}
}

// writeAtomic never replaces an existing destination, even if another process
// creates that path after uniqueOutputPath checked it.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".scompress-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		_ = tmp.Close()
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Link(tmpName, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("output already exists: %s", path)
		}
		return err
	}
	ok = true
	_ = os.Remove(tmpName) // The final link is complete; cleanup is best-effort.
	return nil
}

func printBatchSummary(s batchStats) {
	fmt.Println("\nBatch summary")
	fmt.Printf("  Files processed:       %d\n", s.files)
	fmt.Printf("  Compressed:            %d\n", s.compressed)
	fmt.Printf("  Already under target:  %d\n", s.skipped)
	if s.dryRun > 0 {
		fmt.Printf("  Dry-run previews:      %d\n", s.dryRun)
	}
	fmt.Printf("  Failed:                %d\n", s.failed)
	if s.compressed > 0 {
		saved := s.inputBytes - s.outputBytes
		fmt.Printf("  Original total:        %s\n", humanSize(s.inputBytes))
		fmt.Printf("  Result total:          %s\n", humanSize(s.outputBytes))
		fmt.Printf("  Size reduction:       %s (%.1f%% of input total)\n", humanSize(saved), percentSaved(s.inputBytes, s.outputBytes))
		fmt.Println("  Note: originals are retained; this is not disk space reclaimed.")
	}
}

func percentSaved(original, compressed int64) float64 {
	if original <= 0 {
		return 0
	}
	return 100 * (1 - float64(compressed)/float64(original))
}

func humanSize(n int64) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.2f MB", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1f KB", float64(n)/1_000)
	}
	return fmt.Sprintf("%d B", n)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
