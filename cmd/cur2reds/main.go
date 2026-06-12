package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const (
	generatedPackage        = "assets"
	generatedRendererImport = "github.com/juliusplatzer/reds/renderer"
	generatedVarName        = "AsdexCursors"
	generatedFuncName       = "AsdexCursor"
)

var pngSignature = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

type cursorBitmap struct {
	name    string
	width   int
	height  int
	hotspot [2]int
	pixels  []uint32 // row-major 0xRRGGBBAA
}

type iconDirEntry struct {
	width      int
	height     int
	hotspotX   int
	hotspotY   int
	bytesInRes uint32
	imageOff   uint32
}

func main() {
	inDir := flag.String("in", "", "input directory containing .cur or .cur.zst files")
	outPath := flag.String("out", "", "output generated Go file")
	flag.Parse()

	if *inDir == "" || *outPath == "" {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/cursor2reds -in resources/bitmaps/asdex/cursors -out asdex/assets/cursors.go")
		os.Exit(2)
	}

	if err := run(*inDir, *outPath); err != nil {
		fmt.Fprintln(os.Stderr, "cursor2reds:", err)
		os.Exit(1)
	}
}

func run(inDir, outPath string) error {
	paths, err := cursorInputFiles(inDir)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return fmt.Errorf("no .cur or .cur.zst files found in %s", inDir)
	}

	cursors := make([]cursorBitmap, 0, len(paths))
	seen := make(map[string]bool, len(paths))

	for _, path := range paths {
		cursor, err := decodeCursorFile(path)
		if err != nil {
			return err
		}
		if seen[cursor.name] {
			return fmt.Errorf("duplicate cursor name after sanitizing: %s", cursor.name)
		}
		seen[cursor.name] = true
		cursors = append(cursors, cursor)
	}

	sort.Slice(cursors, func(i, j int) bool {
		return cursors[i].name < cursors[j].name
	})

	if err := writeGo(outPath, cursors); err != nil {
		return err
	}

	fmt.Printf("wrote %s\n", outPath)
	fmt.Print("converted cursors:")
	for _, cursor := range cursors {
		fmt.Printf(" %s(%dx%d hot=%d,%d)", cursor.name, cursor.width, cursor.height, cursor.hotspot[0], cursor.hotspot[1])
	}
	fmt.Println()

	return nil
}

func cursorInputFiles(inDir string) ([]string, error) {
	info, err := os.Stat(inDir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("-in must be a directory: %s", inDir)
	}

	entries, err := os.ReadDir(inDir)
	if err != nil {
		return nil, err
	}

	var paths []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if strings.HasPrefix(name, "._") {
			continue
		}
		if strings.HasSuffix(name, ".cur") || strings.HasSuffix(name, ".cur.zst") {
			paths = append(paths, filepath.Join(inDir, name))
		}
	}

	sort.Strings(paths)
	return paths, nil
}

func decodeCursorFile(path string) (cursorBitmap, error) {
	raw, err := readMaybeZstd(path)
	if err != nil {
		return cursorBitmap{}, err
	}

	if len(raw) < 6 {
		return cursorBitmap{}, fmt.Errorf("%s: too short for CUR header", path)
	}

	reserved := binary.LittleEndian.Uint16(raw[0:2])
	typ := binary.LittleEndian.Uint16(raw[2:4])
	count := binary.LittleEndian.Uint16(raw[4:6])

	if reserved != 0 || typ != 2 {
		return cursorBitmap{}, fmt.Errorf("%s: not a Windows .cur file: reserved=%d type=%d", path, reserved, typ)
	}
	if count == 0 {
		return cursorBitmap{}, fmt.Errorf("%s: no cursor images", path)
	}

	entries := make([]iconDirEntry, 0, count)
	off := 6
	for i := 0; i < int(count); i++ {
		if off+16 > len(raw) {
			return cursorBitmap{}, fmt.Errorf("%s: truncated cursor directory", path)
		}

		width := int(raw[off])
		height := int(raw[off+1])
		if width == 0 {
			width = 256
		}
		if height == 0 {
			height = 256
		}

		entry := iconDirEntry{
			width:      width,
			height:     height,
			hotspotX:   int(binary.LittleEndian.Uint16(raw[off+4 : off+6])),
			hotspotY:   int(binary.LittleEndian.Uint16(raw[off+6 : off+8])),
			bytesInRes: binary.LittleEndian.Uint32(raw[off+8 : off+12]),
			imageOff:   binary.LittleEndian.Uint32(raw[off+12 : off+16]),
		}
		entries = append(entries, entry)
		off += 16
	}

	// Pick the largest cursor image deterministically. The CRC ASDE-X cursor
	// resources currently contain one 32x32 entry each, but this also handles
	// multi-image .cur files.
	sort.Slice(entries, func(i, j int) bool {
		areaI := entries[i].width * entries[i].height
		areaJ := entries[j].width * entries[j].height
		if areaI != areaJ {
			return areaI > areaJ
		}
		return entries[i].bytesInRes > entries[j].bytesInRes
	})

	name := cursorNameFromPath(path)
	cursor, err := decodeCursorImage(raw, entries[0], name)
	if err != nil {
		return cursorBitmap{}, fmt.Errorf("%s: %w", path, err)
	}
	return cursor, nil
}

func readMaybeZstd(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if !strings.HasSuffix(path, ".zst") {
		return raw, nil
	}

	zstdPath, err := exec.LookPath("zstd")
	if err != nil {
		return nil, fmt.Errorf("%s is zstd-compressed; install zstd first, e.g. `brew install zstd`", path)
	}

	cmd := exec.Command(zstdPath, "-q", "-d", "-c", path)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("zstd failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func decodeCursorImage(file []byte, entry iconDirEntry, name string) (cursorBitmap, error) {
	start := int(entry.imageOff)
	end := start + int(entry.bytesInRes)

	if start < 0 || end < start || end > len(file) {
		return cursorBitmap{}, fmt.Errorf("cursor image outside file")
	}

	payload := file[start:end]
	if len(payload) >= len(pngSignature) && bytes.Equal(payload[:len(pngSignature)], pngSignature) {
		return decodePNGCursor(payload, entry.hotspotX, entry.hotspotY, name)
	}

	return decodeDIBCursor(payload, entry.hotspotX, entry.hotspotY, name)
}

func decodePNGCursor(payload []byte, hotX, hotY int, name string) (cursorBitmap, error) {
	img, err := png.Decode(bytes.NewReader(payload))
	if err != nil {
		return cursorBitmap{}, fmt.Errorf("decode PNG cursor: %w", err)
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	pixels := make([]uint32, 0, width*height)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r16, g16, b16, a16 := img.At(x, y).RGBA()
			pixels = append(pixels, rgba(uint8(r16>>8), uint8(g16>>8), uint8(b16>>8), uint8(a16>>8)))
		}
	}

	return cursorBitmap{
		name:    name,
		width:   width,
		height:  height,
		hotspot: [2]int{hotX, hotY},
		pixels:  pixels,
	}, nil
}

func decodeDIBCursor(payload []byte, hotX, hotY int, name string) (cursorBitmap, error) {
	if len(payload) < 40 {
		return cursorBitmap{}, fmt.Errorf("truncated BITMAPINFOHEADER")
	}

	headerSize := int(int32(binary.LittleEndian.Uint32(payload[0:4])))
	if headerSize < 40 {
		return cursorBitmap{}, fmt.Errorf("unsupported DIB header size %d", headerSize)
	}
	if headerSize > len(payload) {
		return cursorBitmap{}, fmt.Errorf("truncated DIB header")
	}

	width := int(int32(binary.LittleEndian.Uint32(payload[4:8])))
	dibHeight := int(int32(binary.LittleEndian.Uint32(payload[8:12])))
	planes := binary.LittleEndian.Uint16(payload[12:14])
	bpp := int(binary.LittleEndian.Uint16(payload[14:16]))
	compression := binary.LittleEndian.Uint32(payload[16:20])
	clrUsed := int(binary.LittleEndian.Uint32(payload[32:36]))

	if width <= 0 || dibHeight == 0 {
		return cursorBitmap{}, fmt.Errorf("invalid DIB dimensions %dx%d", width, dibHeight)
	}
	if planes != 1 {
		return cursorBitmap{}, fmt.Errorf("expected one plane, got %d", planes)
	}
	if compression != 0 {
		return cursorBitmap{}, fmt.Errorf("compressed DIB cursors are not supported, compression=%d", compression)
	}

	// In .cur DIB payloads, biHeight includes XOR bitmap plus AND mask.
	height := abs(dibHeight) / 2
	topDown := dibHeight < 0
	if height <= 0 {
		return cursorBitmap{}, fmt.Errorf("invalid cursor bitmap height %d", dibHeight)
	}

	pos := headerSize

	var palette [][4]uint8 // r,g,b,a
	if bpp <= 8 {
		ncolors := clrUsed
		if ncolors == 0 {
			ncolors = 1 << bpp
		}
		if pos+ncolors*4 > len(payload) {
			return cursorBitmap{}, fmt.Errorf("truncated palette")
		}

		palette = make([][4]uint8, ncolors)
		for i := 0; i < ncolors; i++ {
			b := payload[pos+i*4+0]
			g := payload[pos+i*4+1]
			r := payload[pos+i*4+2]
			palette[i] = [4]uint8{r, g, b, 255}
		}
		pos += ncolors * 4
	}

	xorStride := dibStride(width, bpp)
	andStride := dibStride(width, 1)
	xorBytes := xorStride * height
	andBytes := andStride * height

	if pos+xorBytes+andBytes > len(payload) {
		return cursorBitmap{}, fmt.Errorf("truncated bitmap data")
	}

	xorStart := pos
	andStart := pos + xorBytes

	pixels := make([]uint32, width*height)
	hasRealAlpha := false

	// Decode the XOR bitmap first, preserving 32-bit alpha when present.
	for y := 0; y < height; y++ {
		srcY := height - 1 - y
		if topDown {
			srcY = y
		}

		xorRow := payload[xorStart+srcY*xorStride : xorStart+(srcY+1)*xorStride]

		for x := 0; x < width; x++ {
			var r, g, b, a uint8

			switch bpp {
			case 32:
				i := x * 4
				b, g, r, a = xorRow[i], xorRow[i+1], xorRow[i+2], xorRow[i+3]
				if a != 0 {
					hasRealAlpha = true
				}
			case 24:
				i := x * 3
				b, g, r = xorRow[i], xorRow[i+1], xorRow[i+2]
				a = 255
			case 8, 4, 1:
				idx := decodeIndexedPixel(xorRow, x, bpp)
				if idx < len(palette) {
					r, g, b, a = palette[idx][0], palette[idx][1], palette[idx][2], palette[idx][3]
				} else {
					r, g, b, a = 0, 0, 0, 255
				}
			default:
				return cursorBitmap{}, fmt.Errorf("unsupported bits-per-pixel value %d", bpp)
			}

			pixels[y*width+x] = rgba(r, g, b, a)
		}
	}

	// Apply the AND transparency mask. For old-style 32-bit cursors without a
	// real alpha channel, make every non-masked pixel opaque. For modern CRC
	// ASDE-X cursors with real alpha, preserve the exact antialiased alpha.
	for y := 0; y < height; y++ {
		srcY := height - 1 - y
		if topDown {
			srcY = y
		}

		andRow := payload[andStart+srcY*andStride : andStart+(srcY+1)*andStride]

		for x := 0; x < width; x++ {
			px := pixels[y*width+x]
			a := uint8(px & 0xff)

			if bpp == 32 && !hasRealAlpha {
				a = 255
			} else if bpp != 32 {
				a = 255
			}

			if maskBit(andRow, x) != 0 {
				a = 0
			}

			pixels[y*width+x] = (px & 0xffffff00) | uint32(a)
		}
	}

	return cursorBitmap{
		name:    name,
		width:   width,
		height:  height,
		hotspot: [2]int{hotX, hotY},
		pixels:  pixels,
	}, nil
}

func dibStride(width, bitsPerPixel int) int {
	return ((width*bitsPerPixel + 31) / 32) * 4
}

func decodeIndexedPixel(row []byte, x int, bpp int) int {
	switch bpp {
	case 8:
		return int(row[x])
	case 4:
		v := row[x/2]
		if x%2 == 0 {
			return int(v >> 4)
		}
		return int(v & 0x0f)
	case 1:
		return maskBit(row, x)
	default:
		panic("unsupported indexed bpp")
	}
}

func maskBit(row []byte, x int) int {
	v := row[x/8]
	shift := 7 - (x % 8)
	return int((v >> shift) & 1)
}

func rgba(r, g, b, a uint8) uint32 {
	return uint32(r)<<24 | uint32(g)<<16 | uint32(b)<<8 | uint32(a)
}

func cursorNameFromPath(path string) string {
	base := filepath.Base(path)
	if strings.HasSuffix(base, ".cur.zst") {
		base = strings.TrimSuffix(base, ".cur.zst")
	} else if strings.HasSuffix(base, ".cur") {
		base = strings.TrimSuffix(base, ".cur")
	}

	var b strings.Builder
	lastUnderscore := false
	for _, r := range base {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastUnderscore = false
		} else if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}

	name := strings.Trim(b.String(), "_")
	if name == "" {
		name = "Cursor"
	}
	return name
}

func writeGo(outPath string, cursors []cursorBitmap) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString("// Code generated by cmd/cursor2reds; DO NOT EDIT.\n")
	b.WriteString("// Source: ASDE-X .cur(.zst) files converted to exact RGBA cursor pixels.\n\n")
	fmt.Fprintf(&b, "package %s\n\n", generatedPackage)
	fmt.Fprintf(&b, "import renderer %q\n\n", generatedRendererImport)

	fmt.Fprintf(&b, "var %s = map[string]*renderer.CursorBitmap{\n", generatedVarName)

	for _, cursor := range cursors {
		fmt.Fprintf(&b, "\t%q: {\n", cursor.name)
		fmt.Fprintf(&b, "\t\tName:    %q,\n", cursor.name)
		fmt.Fprintf(&b, "\t\tWidth:   %d,\n", cursor.width)
		fmt.Fprintf(&b, "\t\tHeight:  %d,\n", cursor.height)
		fmt.Fprintf(&b, "\t\tHotspot: [2]int{%d, %d},\n", cursor.hotspot[0], cursor.hotspot[1])
		fmt.Fprintf(&b, "\t\tPixels:  %s,\n", formatUint32Array(cursor.pixels, "\t\t"))
		b.WriteString("\t},\n\n")
	}

	b.WriteString("}\n\n")
	fmt.Fprintf(&b, "func %s(name string) (*renderer.CursorBitmap, bool) {\n", generatedFuncName)
	fmt.Fprintf(&b, "\tc, ok := %s[name]\n", generatedVarName)
	b.WriteString("\treturn c, ok\n")
	b.WriteString("}\n")

	return os.WriteFile(outPath, []byte(b.String()), 0o644)
}

func formatUint32Array(values []uint32, indent string) string {
	if len(values) == 0 {
		return "nil"
	}

	const perLine = 8

	var b strings.Builder
	b.WriteString("[]uint32{")
	for i := 0; i < len(values); i += perLine {
		end := i + perLine
		if end > len(values) {
			end = len(values)
		}

		b.WriteString("\n")
		b.WriteString(indent)
		for j := i; j < end; j++ {
			if j > i {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "0x%08X", values[j])
		}
		b.WriteString(",")
	}
	b.WriteString("\n")
	b.WriteString(indent[:len(indent)-1])
	b.WriteString("}")

	return b.String()
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
