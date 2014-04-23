package sgreader

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"io"
)

const (
	ISOMETRIC_TILE_WIDTH        = 58
	ISOMETRIC_TILE_HEIGHT       = 30
	ISOMETRIC_TILE_BYTES        = 1800
	ISOMETRIC_LARGE_TILE_WIDTH  = 78
	ISOMETRIC_LARGE_TILE_HEIGHT = 40
	ISOMETRIC_LARGE_TILE_BYTES  = 3200
)

type SgImageRecord struct {
	Offset             uint32
	Length             uint32
	UncompressedLength uint32
	_                  [4]byte
	InvertOffset       int32
	Width              int16
	Height             int16
	_                  [26]byte
	Type               uint16
	Flags              [4]uint8
	BitmapId           uint8
	_                  [7]byte
	AlphaOffset        uint32
	AlphaLength        uint32
}

type SgImageRecordNonAlpha struct {
	Offset             uint32
	Length             uint32
	UncompressedLength uint32
	_                  [4]byte
	InvertOffset       int32
	Width              int16
	Height             int16
	_                  [26]byte
	Type               uint16
	Flags              [4]uint8
	BitmapId           uint8
	_                  [7]byte
}

func (s *SgImageRecordNonAlpha) convert() *SgImageRecord {
	return &SgImageRecord{
		Offset:             s.Offset,
		Length:             s.Length,
		UncompressedLength: s.UncompressedLength,
		InvertOffset:       s.InvertOffset,
		Width:              s.Width,
		Height:             s.Height,
		Type:               s.Type,
		Flags:              s.Flags,
		BitmapId:           s.BitmapId,
	}
}

func newImageRecord(r io.Reader, includeAlpha bool) (*SgImageRecord, error) {
	if includeAlpha {
		record := &SgImageRecord{}
		err := binary.Read(r, binary.LittleEndian, record)
		return record, err
	}

	record := &SgImageRecordNonAlpha{}
	err := binary.Read(r, binary.LittleEndian, record)
	if err != nil {
		return nil, err
	}
	return record.convert(), nil
}

// SgImage stores the metadata of the image
type SgImage struct {
	record     *SgImageRecord
	workRecord *SgImageRecord
	parent     *SgBitmap
	invert     bool
	imageId    int
}

func newSgImage(id int, r io.Reader, includeAlpha bool) (*SgImage, error) {
	record, err := newImageRecord(r, includeAlpha)
	if err != nil {
		return nil, err
	}
	workRecord := record
	invert := false
	if record.InvertOffset > 0 {
		invert = true
	}
	return &SgImage{
		record:     record,
		workRecord: workRecord,
		invert:     invert,
		imageId:    id,
	}, nil
}

// Retrieves the invert offset
func (sgImage *SgImage) InvertOffset() int32 {
	return sgImage.record.InvertOffset
}

// The ID of the image within the bitmap
func (sgImage *SgImage) BitmapId() int {
	if sgImage.workRecord != nil {
		return int(sgImage.workRecord.BitmapId)
	}
	return int(sgImage.record.BitmapId)
}

// Returns the width and height of this image
func (sgImage *SgImage) String() string {
	return fmt.Sprintf("%dx%d", int(sgImage.workRecord.Width), int(sgImage.workRecord.Height))
}

// Returns the full information of this image
func (sgImage *SgImage) FullDescription() string {
	flag := "internal"
	if sgImage.workRecord.Flags[0] != 0 {
		flag = "external"
	}
	return fmt.Sprintf("ID %d: offset %d, length %d, width %d, height %d, type %d, %s", sgImage.imageId, sgImage.workRecord.Offset, sgImage.workRecord.Length, sgImage.workRecord.Width, sgImage.workRecord.Height, sgImage.workRecord.Type, flag)
}

// Set the work record of the inverted image
func (sgImage *SgImage) SetInvertImage(invert *SgImage) {
	sgImage.workRecord = invert.record
}

// Set the parent bitmap of the image
func (sgImage *SgImage) SetParent(parent *SgBitmap) {
	sgImage.parent = parent
}

// Get the image.RGBA object for this image
func (sgImage *SgImage) GetImage() (*image.RGBA, error) {
	if sgImage.parent == nil {
		return nil, errors.New("Image has no bitmap parent")
	}
	if sgImage.workRecord.Width <= 0 || sgImage.workRecord.Height <= 0 {
		return nil, fmt.Errorf("Width or height invalid (%dx%d)", sgImage.workRecord.Width, sgImage.workRecord.Height)
	} else if sgImage.workRecord.Length <= 0 {
		return nil, errors.New("No image data available")
	}

	buffer, err := sgImage.fillBuffer()
	if err != nil {
		return nil, err
	}

	result := image.NewRGBA(image.Rect(0, 0, int(sgImage.workRecord.Width), int(sgImage.workRecord.Height)))
	// Initialize image to transparent black
	draw.Draw(result, result.Bounds(), &image.Uniform{color.RGBA{0, 0, 0, 0}}, image.ZP, draw.Src)

	switch sgImage.workRecord.Type {
	case 0, 1, 10, 12, 13:
		err = sgImage.loadPlainImage(result, buffer)
	case 30:
		err = sgImage.loadIsometricImage(result, buffer)
	case 256, 257, 276:
		sgImage.loadSpriteImage(result, buffer)
	default:
		return nil, fmt.Errorf("Unknown image type: %d", sgImage.workRecord.Type)
	}
	if err != nil {
		return nil, err
	}

	if sgImage.workRecord.AlphaLength > 0 {
		alphaBuffer := buffer[sgImage.workRecord.Length:]
		sgImage.loadAlphaMask(result, alphaBuffer)
	}

	if sgImage.invert {
		mirrored := image.NewRGBA(result.Bounds())
		for y := 0; y <= result.Bounds().Dy(); y++ {
			for x := 0; x <= result.Bounds().Dx()/2; x++ {
				mirrored.Set(x, y, result.At(result.Bounds().Dx()-x, y))
				mirrored.Set(result.Bounds().Dx()-x, y, result.At(x, y))
			}
		}
		return mirrored, nil
	}
	return result, nil
}

func (sgImage *SgImage) fillBuffer() ([]byte, error) {
	if sgImage.parent == nil {
		return nil, errors.New("Image has no bitmap parent")
	}
	file, err := sgImage.parent.OpenFile(sgImage.workRecord.Flags[0] != 0)
	if err != nil {
		return nil, err
	}

	dataLength := sgImage.workRecord.Length + sgImage.workRecord.AlphaLength
	if dataLength <= 0 {
		fmt.Printf("Data length: %d\n", dataLength)
	}
	buffer := make([]byte, dataLength)

	if sgImage.workRecord.Flags[0] != 0 {
		_, err := file.Seek(int64(sgImage.workRecord.Offset)-1, 0)
		if err != nil {
			return nil, err
		}
	} else {
		_, err := file.Seek(int64(sgImage.workRecord.Offset), 0)
		if err != nil {
			return nil, err
		}
	}

	dataRead, err := file.Read(buffer)
	if int(dataLength) != dataRead || err != nil {
		if dataRead+4 == int(dataLength) {
			buffer[dataRead] = 0
			buffer[dataRead+1] = 0
			buffer[dataRead+2] = 0
			buffer[dataRead+3] = 0
		} else {
			return nil, fmt.Errorf("Unable to read %d bytes from file (read %d bytes)", dataLength, dataRead)
		}
	}

	return buffer, nil
}

func (sgImage *SgImage) loadPlainImage(img *image.RGBA, buffer []byte) error {
	if int(sgImage.workRecord.Height)*int(sgImage.workRecord.Width)*2 != int(sgImage.workRecord.Length) {
		return errors.New("Image data length doesn't match image size")
	}

	i := 0
	for y := 0; y < int(sgImage.workRecord.Height); y++ {
		for x := 0; x < int(sgImage.workRecord.Width); x++ {
			sgImage.set555Pixel(img, x, y, uint16(buffer[i])|uint16(buffer[i]<<8))
			i += 2
		}
	}
	return nil
}

func (sgImage *SgImage) loadIsometricImage(img *image.RGBA, buffer []byte) error {
	err := sgImage.writeIsometricBase(img, buffer)
	if err != nil {
		return err
	}
	sgImage.writeTransparentImage(img, buffer[sgImage.workRecord.UncompressedLength:], int(sgImage.workRecord.Length-sgImage.workRecord.UncompressedLength))
	return nil
}

func (sgImage *SgImage) loadSpriteImage(img *image.RGBA, buffer []byte) {
	sgImage.writeTransparentImage(img, buffer, int(sgImage.workRecord.Length))
}

func (sgImage *SgImage) loadAlphaMask(img *image.RGBA, buffer []byte) {
	width := img.Bounds().Dx()
	length := int(sgImage.workRecord.AlphaLength)
	var i, x, y int

	for i < length {
		c := int(buffer[i])
		i++
		if c == 255 {
			// The next byte is the number of pixels to skip
			x += int(buffer[i])
			i++
			for x >= width {
				y++
				x -= width
			}
		} else {
			// 'c' is the number of image data bytes
			for j := 0; j < c; j++ {
				sgImage.setAlphaPixel(img, x, y, buffer[i])
				x++
				if x >= width {
					y++
					x = 0
				}
				i += 2
			}
		}
	}
}

func (sgImage *SgImage) writeIsometricBase(img *image.RGBA, buffer []byte) error {
	width := img.Bounds().Dx()
	height := (width + 2) / 2 /* 58 -> 30, 118 -> 60, etc */
	heightOffset := img.Bounds().Dy() - height
	var size int
	size = int(sgImage.workRecord.Flags[3])
	yOffset := heightOffset
	var xOffset, tileBytes, tileHeight, tileWidth int

	if size == 0 {
		/* Derive the tile size from the height (more regular than width)
		 * Note that this causes a problem with 4x4 regular vs 3x3 large:
		 * 4 * 30 = 120; 3 * 40 = 120 -- give precedence to regular */
		if height%ISOMETRIC_TILE_HEIGHT == 0 {
			size = height / ISOMETRIC_TILE_HEIGHT
		} else if height%ISOMETRIC_LARGE_TILE_HEIGHT == 0 {
			size = height / ISOMETRIC_LARGE_TILE_HEIGHT
		}
	}

	// Determine whether we should use the regular or large (emperor) tiles
	if ISOMETRIC_TILE_HEIGHT*size == height {
		// Regular tile
		tileBytes = ISOMETRIC_TILE_BYTES
		tileHeight = ISOMETRIC_TILE_HEIGHT
		tileWidth = ISOMETRIC_TILE_WIDTH
	} else if ISOMETRIC_LARGE_TILE_HEIGHT*size == height {
		// Large (emperor) tile
		tileBytes = ISOMETRIC_LARGE_TILE_BYTES
		tileHeight = ISOMETRIC_LARGE_TILE_HEIGHT
		tileWidth = ISOMETRIC_LARGE_TILE_WIDTH
	} else {
		return fmt.Errorf("Unknown tile size: %d (height %d, width %d, size %d)", 2*height/size, height, width, size)
	}

	// Check if buffer length is enough: (width + 2) * height / 2 * 2bpp
	if (width+2)*height != int(sgImage.workRecord.UncompressedLength) {
		return fmt.Errorf("Data length doesn't match footprint size: %d vs %d (%d) %d", (width+2)*height, sgImage.workRecord.UncompressedLength, sgImage.workRecord.Length, sgImage.workRecord.InvertOffset)
	}

	i := 0
	for y := 0; y < (size + (size - 1)); y++ {
		var xRange int
		if y < size {
			xOffset = size - y - 1
			xRange = y + 1
		} else {
			xOffset = y - size + 1
			xRange = 2*size - y - 1
		}
		xOffset *= tileHeight
		for x := 0; x < xRange; x++ {
			sgImage.writeIsometricTile(img, buffer[i*tileBytes:], xOffset, yOffset, tileWidth, tileHeight)
			xOffset += tileWidth + 2
			i++
		}
		yOffset += tileHeight / 2
	}
	return nil
}

func (sgImage *SgImage) writeIsometricTile(img *image.RGBA, buffer []byte, xOffset, yOffset, tileWidth, tileHeight int) {
	halfHeight := tileHeight / 2
	i := 0
	for y := 0; y < halfHeight; y++ {
		start := tileHeight - 2*(y+1)
		end := tileWidth - start
		for x := start; x < end; x++ {
			sgImage.set555Pixel(img, xOffset+x, yOffset+y, uint16(buffer[i+1]<<8)|uint16(buffer[i]))
			i += 2
		}
	}
	for y := halfHeight; y < tileHeight; y++ {
		start := 2*y - tileHeight
		end := tileWidth - start
		for x := start; x < end; x++ {
			sgImage.set555Pixel(img, xOffset+x, yOffset+y, uint16(buffer[i+1]<<8)|uint16(buffer[i]))
			i += 2
		}
	}
}

func (sgImage *SgImage) writeTransparentImage(img *image.RGBA, buffer []byte, length int) {
	width := img.Bounds().Dx()

	var i, x, y int

	for i < length {
		c := int(buffer[i])
		i++
		if c == 255 {
			// The next byte is the number of pixels to skip
			x += int(buffer[i])
			i++
			for x >= width {
				y++
				x -= width
			}
		} else {
			// 'c' is the number of image data bytes
			for j := 0; j < c; j++ {
				sgImage.set555Pixel(img, x, y, uint16(buffer[i+1]<<8)|uint16(buffer[i]))
				x++
				if x >= width {
					y++
					x = 0
				}
				i += 2
			}
		}
	}
}

func (sgImage *SgImage) set555Pixel(img *image.RGBA, x, y int, c uint16) {
	if c == 0xf81f {
		return
	}

	var rgb uint32
	rgb = 0xff000000

	// Red: bits 11-15, should go to bits 17-24
	rgb |= uint32((c&0x7c00)<<9) | uint32((c&0x7000)<<4)

	// Green: bits 6-10, should go to bits 9-16
	rgb |= uint32((c&0x3e0)<<6) | uint32(c&0x300)

	// Blue: bits 1-5, should go to bits 1-8
	rgb |= uint32((c&0x1f)<<3) | uint32((c&0x1c)>>2)

	img.Set(x, y, color.RGBA{uint8(rgb & 0x000000ff), uint8((rgb & 0x0000ff00) >> 8), uint8((rgb & 0x00ff0000) >> 16), 255})
}

func (sgImage *SgImage) setAlphaPixel(img *image.RGBA, x, y int, c2 uint8) {
	alpha := ((c2 & 0x1f) << 3) | ((c2 & 0x1c) >> 2)
	c := img.At(x, y)
	r, g, b, _ := c.RGBA()
	img.Set(x, y, color.RGBA{uint8(r), uint8(g), uint8(b), alpha})
}
