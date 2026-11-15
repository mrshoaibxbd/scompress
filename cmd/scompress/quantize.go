package main

import (
	"image"
	"image/color"
	"sort"
)

type histogramColor struct {
	r, g, b uint8
	count   int64
	sumR    int64
	sumG    int64
	sumB    int64
}

type colorBox struct {
	items                  []histogramColor
	count                  int64
	rangeR, rangeG, rangeB int
}

// toPaletted creates an image-specific median-cut palette. It handles binary
// transparency (the formats that call it only support fully transparent or
// fully opaque pixels) and avoids a fixed web-safe palette.
func toPaletted(img image.Image, maxColors int) *image.Paletted {
	bounds := img.Bounds()
	transparent := false
	histogram := make(map[uint16]*histogramColor, 32768)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if c.A == 0 {
				transparent = true
				continue
			}
			key := uint16(c.R>>3)<<10 | uint16(c.G>>3)<<5 | uint16(c.B>>3)
			entry := histogram[key]
			if entry == nil {
				entry = &histogramColor{}
				histogram[key] = entry
			}
			entry.count++
			entry.sumR += int64(c.R)
			entry.sumG += int64(c.G)
			entry.sumB += int64(c.B)
		}
	}

	items := make([]histogramColor, 0, len(histogram))
	for _, entry := range histogram {
		entry.r = uint8(entry.sumR / entry.count)
		entry.g = uint8(entry.sumG / entry.count)
		entry.b = uint8(entry.sumB / entry.count)
		items = append(items, *entry)
	}

	colorLimit := maxColors
	if transparent {
		colorLimit--
	}
	if colorLimit < 1 {
		colorLimit = 1
	}
	colors := medianCut(items, colorLimit)
	pal := make(color.Palette, 0, len(colors)+1)
	if transparent {
		pal = append(pal, color.NRGBA{A: 0})
	}
	for _, c := range colors {
		pal = append(pal, color.NRGBA{R: c.r, G: c.g, B: c.b, A: 255})
	}
	if len(pal) == 0 {
		pal = append(pal, color.NRGBA{A: 255})
	}

	// Precompute nearest palette entries for every 5-bit RGB bin. This keeps
	// pixel mapping fast even for multi-megapixel photographs.
	lookup := make([]uint8, 1<<15)
	paletteStart := 0
	if transparent {
		paletteStart = 1
	}
	for key := 0; key < len(lookup); key++ {
		r := uint8(((key>>10)&31)*8 + 4)
		g := uint8(((key>>5)&31)*8 + 4)
		b := uint8((key&31)*8 + 4)
		bestIndex := paletteStart
		bestDistance := int64(1<<62 - 1)
		for i := paletteStart; i < len(pal); i++ {
			p := color.NRGBAModel.Convert(pal[i]).(color.NRGBA)
			dr := int64(int(r) - int(p.R))
			dg := int64(int(g) - int(p.G))
			db := int64(int(b) - int(p.B))
			distance := 30*dr*dr + 59*dg*dg + 11*db*db
			if distance < bestDistance {
				bestDistance = distance
				bestIndex = i
			}
		}
		lookup[key] = uint8(bestIndex)
	}

	out := image.NewPaletted(bounds, pal)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if transparent && c.A == 0 {
				out.SetColorIndex(x, y, 0)
				continue
			}
			key := uint16(c.R>>3)<<10 | uint16(c.G>>3)<<5 | uint16(c.B>>3)
			out.SetColorIndex(x, y, lookup[key])
		}
	}
	return out
}

func medianCut(items []histogramColor, limit int) []histogramColor {
	if len(items) == 0 {
		return []histogramColor{{r: 0, g: 0, b: 0, count: 1}}
	}
	if len(items) <= limit {
		return items
	}
	boxes := []colorBox{makeBox(items)}
	for len(boxes) < limit {
		best := -1
		bestScore := int64(-1)
		for i, box := range boxes {
			if len(box.items) < 2 {
				continue
			}
			spread := maxInt(box.rangeR, maxInt(box.rangeG, box.rangeB))
			score := int64(spread) * box.count
			if score > bestScore {
				bestScore, best = score, i
			}
		}
		if best < 0 {
			break
		}
		left, right := splitBox(boxes[best])
		boxes[best] = left
		boxes = append(boxes, right)
	}

	out := make([]histogramColor, 0, len(boxes))
	for _, box := range boxes {
		var sumR, sumG, sumB, total int64
		for _, c := range box.items {
			sumR += c.sumR
			sumG += c.sumG
			sumB += c.sumB
			total += c.count
		}
		if total == 0 {
			continue
		}
		out = append(out, histogramColor{
			r:     uint8(sumR / total),
			g:     uint8(sumG / total),
			b:     uint8(sumB / total),
			count: total,
		})
	}
	return out
}

func makeBox(items []histogramColor) colorBox {
	box := colorBox{items: items}
	minR, minG, minB := 255, 255, 255
	maxR, maxG, maxB := 0, 0, 0
	for _, c := range items {
		if int(c.r) < minR {
			minR = int(c.r)
		}
		if int(c.g) < minG {
			minG = int(c.g)
		}
		if int(c.b) < minB {
			minB = int(c.b)
		}
		if int(c.r) > maxR {
			maxR = int(c.r)
		}
		if int(c.g) > maxG {
			maxG = int(c.g)
		}
		if int(c.b) > maxB {
			maxB = int(c.b)
		}
		box.count += c.count
	}
	box.rangeR, box.rangeG, box.rangeB = maxR-minR, maxG-minG, maxB-minB
	return box
}

func splitBox(box colorBox) (colorBox, colorBox) {
	channel := 0
	if box.rangeG > box.rangeR && box.rangeG >= box.rangeB {
		channel = 1
	} else if box.rangeB > box.rangeR && box.rangeB > box.rangeG {
		channel = 2
	}
	sort.Slice(box.items, func(i, j int) bool {
		switch channel {
		case 1:
			return box.items[i].g < box.items[j].g
		case 2:
			return box.items[i].b < box.items[j].b
		default:
			return box.items[i].r < box.items[j].r
		}
	})
	mid := len(box.items) / 2
	var accumulated int64
	half := box.count / 2
	for i, c := range box.items {
		accumulated += c.count
		if accumulated >= half && i+1 < len(box.items) {
			mid = i + 1
			break
		}
	}
	return makeBox(box.items[:mid]), makeBox(box.items[mid:])
}

type rgbaHistogramColor struct {
	r, g, b, a             uint8
	count                  int64
	sumR, sumG, sumB, sumA int64
}

type rgbaColorBox struct {
	items  []rgbaHistogramColor
	count  int64
	ranges [4]int
}

// toPalettedRGBA creates an adaptive palette that retains partial alpha,
// which matters for transparent PNGs with anti-aliased edges.
func toPalettedRGBA(img image.Image, maxColors int) *image.Paletted {
	bounds := img.Bounds()
	histogram := make(map[uint32]*rgbaHistogramColor, 1<<16)
	hasTransparent := false
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if c.A == 0 {
				hasTransparent = true
				continue
			}
			key := uint32(c.R>>3)<<13 | uint32(c.G>>3)<<8 | uint32(c.B>>3)<<3 | uint32(c.A>>5)
			entry := histogram[key]
			if entry == nil {
				entry = &rgbaHistogramColor{}
				histogram[key] = entry
			}
			entry.count++
			entry.sumR += int64(c.R)
			entry.sumG += int64(c.G)
			entry.sumB += int64(c.B)
			entry.sumA += int64(c.A)
		}
	}

	items := make([]rgbaHistogramColor, 0, len(histogram))
	for _, entry := range histogram {
		entry.r = uint8(entry.sumR / entry.count)
		entry.g = uint8(entry.sumG / entry.count)
		entry.b = uint8(entry.sumB / entry.count)
		entry.a = uint8(entry.sumA / entry.count)
		items = append(items, *entry)
	}
	colorLimit := maxColors
	if hasTransparent {
		colorLimit--
	}
	if colorLimit < 1 {
		colorLimit = 1
	}
	colors := medianCutRGBA(items, colorLimit)
	pal := make(color.Palette, 0, len(colors)+1)
	if hasTransparent {
		pal = append(pal, color.NRGBA{A: 0})
	}
	for _, c := range colors {
		pal = append(pal, color.NRGBA{R: c.r, G: c.g, B: c.b, A: c.a})
	}
	paletteStart := 0
	if hasTransparent {
		paletteStart = 1
	}

	lookup := make([]uint8, 1<<18)
	for key := 0; key < len(lookup); key++ {
		r := uint8(((key>>13)&31)*8 + 4)
		g := uint8(((key>>8)&31)*8 + 4)
		b := uint8(((key>>3)&31)*8 + 4)
		a := uint8((key&7)*32 + 16)
		bestIndex := paletteStart
		bestDistance := int64(1<<62 - 1)
		for i := paletteStart; i < len(pal); i++ {
			p := color.NRGBAModel.Convert(pal[i]).(color.NRGBA)
			dr := int64(int(r) - int(p.R))
			dg := int64(int(g) - int(p.G))
			db := int64(int(b) - int(p.B))
			da := int64(int(a) - int(p.A))
			distance := 30*dr*dr + 59*dg*dg + 11*db*db + 50*da*da
			if distance < bestDistance {
				bestDistance, bestIndex = distance, i
			}
		}
		lookup[key] = uint8(bestIndex)
	}

	out := image.NewPaletted(bounds, pal)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if hasTransparent && c.A == 0 {
				out.SetColorIndex(x, y, 0)
				continue
			}
			key := uint32(c.R>>3)<<13 | uint32(c.G>>3)<<8 | uint32(c.B>>3)<<3 | uint32(c.A>>5)
			out.SetColorIndex(x, y, lookup[key])
		}
	}
	return out
}

func medianCutRGBA(items []rgbaHistogramColor, limit int) []rgbaHistogramColor {
	if len(items) <= limit {
		return items
	}
	boxes := []rgbaColorBox{makeRGBBox(items)}
	for len(boxes) < limit {
		best := -1
		bestScore := int64(-1)
		for i, box := range boxes {
			if len(box.items) < 2 {
				continue
			}
			spread := maxInt(box.ranges[0], maxInt(box.ranges[1], maxInt(box.ranges[2], box.ranges[3])))
			score := int64(spread) * box.count
			if score > bestScore {
				best, bestScore = i, score
			}
		}
		if best < 0 {
			break
		}
		left, right := splitRGBBox(boxes[best])
		boxes[best] = left
		boxes = append(boxes, right)
	}

	out := make([]rgbaHistogramColor, 0, len(boxes))
	for _, box := range boxes {
		var sr, sg, sb, sa, total int64
		for _, c := range box.items {
			sr += c.sumR
			sg += c.sumG
			sb += c.sumB
			sa += c.sumA
			total += c.count
		}
		if total == 0 {
			continue
		}
		out = append(out, rgbaHistogramColor{
			r: uint8(sr / total), g: uint8(sg / total), b: uint8(sb / total), a: uint8(sa / total), count: total,
		})
	}
	return out
}

func makeRGBBox(items []rgbaHistogramColor) rgbaColorBox {
	box := rgbaColorBox{items: items}
	mins := [4]int{255, 255, 255, 255}
	maxs := [4]int{0, 0, 0, 0}
	for _, c := range items {
		v := [4]int{int(c.r), int(c.g), int(c.b), int(c.a)}
		for i := 0; i < 4; i++ {
			if v[i] < mins[i] {
				mins[i] = v[i]
			}
			if v[i] > maxs[i] {
				maxs[i] = v[i]
			}
		}
		box.count += c.count
	}
	for i := 0; i < 4; i++ {
		box.ranges[i] = maxs[i] - mins[i]
	}
	return box
}

func splitRGBBox(box rgbaColorBox) (rgbaColorBox, rgbaColorBox) {
	channel := 0
	for i := 1; i < 4; i++ {
		if box.ranges[i] > box.ranges[channel] {
			channel = i
		}
	}
	sort.Slice(box.items, func(i, j int) bool {
		a := []uint8{box.items[i].r, box.items[i].g, box.items[i].b, box.items[i].a}
		b := []uint8{box.items[j].r, box.items[j].g, box.items[j].b, box.items[j].a}
		return a[channel] < b[channel]
	})
	mid := len(box.items) / 2
	var accumulated int64
	half := box.count / 2
	for i, c := range box.items {
		accumulated += c.count
		if accumulated >= half && i+1 < len(box.items) {
			mid = i + 1
			break
		}
	}
	return makeRGBBox(box.items[:mid]), makeRGBBox(box.items[mid:])
}

// toPalettedFrames builds one shared palette for every GIF frame. A common
// palette both avoids color flicker and is substantially faster than running
// the full median-cut and nearest-color search independently per frame.
func toPalettedFrames(frames []image.Image, maxColors int) []*image.Paletted {
	if len(frames) == 0 {
		return nil
	}
	histogram := make(map[uint16]*histogramColor, 32768)
	transparent := false
	for _, img := range frames {
		bounds := img.Bounds()
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
				if c.A == 0 {
					transparent = true
					continue
				}
				key := uint16(c.R>>3)<<10 | uint16(c.G>>3)<<5 | uint16(c.B>>3)
				entry := histogram[key]
				if entry == nil {
					entry = &histogramColor{}
					histogram[key] = entry
				}
				entry.count++
				entry.sumR += int64(c.R)
				entry.sumG += int64(c.G)
				entry.sumB += int64(c.B)
			}
		}
	}

	items := make([]histogramColor, 0, len(histogram))
	for _, entry := range histogram {
		entry.r = uint8(entry.sumR / entry.count)
		entry.g = uint8(entry.sumG / entry.count)
		entry.b = uint8(entry.sumB / entry.count)
		items = append(items, *entry)
	}
	colorLimit := maxColors
	if transparent {
		colorLimit--
	}
	if colorLimit < 1 {
		colorLimit = 1
	}
	colors := medianCut(items, colorLimit)
	palette := make(color.Palette, 0, len(colors)+1)
	if transparent {
		palette = append(palette, color.NRGBA{A: 0})
	}
	for _, c := range colors {
		palette = append(palette, color.NRGBA{R: c.r, G: c.g, B: c.b, A: 255})
	}
	if len(palette) == 0 {
		palette = append(palette, color.NRGBA{A: 255})
	}

	lookup := make([]uint8, 1<<15)
	paletteStart := 0
	if transparent {
		paletteStart = 1
	}
	for key := 0; key < len(lookup); key++ {
		r := uint8(((key>>10)&31)*8 + 4)
		g := uint8(((key>>5)&31)*8 + 4)
		b := uint8((key&31)*8 + 4)
		bestIndex := paletteStart
		bestDistance := int64(1<<62 - 1)
		for i := paletteStart; i < len(palette); i++ {
			p := color.NRGBAModel.Convert(palette[i]).(color.NRGBA)
			dr := int64(int(r) - int(p.R))
			dg := int64(int(g) - int(p.G))
			db := int64(int(b) - int(p.B))
			distance := 30*dr*dr + 59*dg*dg + 11*db*db
			if distance < bestDistance {
				bestDistance, bestIndex = distance, i
			}
		}
		lookup[key] = uint8(bestIndex)
	}

	out := make([]*image.Paletted, len(frames))
	for frameIndex, img := range frames {
		bounds := img.Bounds()
		paletted := image.NewPaletted(bounds, palette)
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
				if transparent && c.A == 0 {
					paletted.SetColorIndex(x, y, 0)
					continue
				}
				key := uint16(c.R>>3)<<10 | uint16(c.G>>3)<<5 | uint16(c.B>>3)
				paletted.SetColorIndex(x, y, lookup[key])
			}
		}
		out[frameIndex] = paletted
	}
	return out
}
