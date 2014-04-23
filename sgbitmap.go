// Package sgreader provides functions for reading SG files which are used by
// City Builder games (Zeus, Caesar 3, Pharaoh etc) to store art assets.
package sgreader

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	recordSize int = 200
)

type bitmapRecord struct {
	filename   [65]byte
	comment    [51]byte
	width      uint32
	height     uint32
	numImages  uint32
	startIndex uint32
	endIndex   uint32
	_          [64]byte
}

func (s *bitmapRecord) filenameString() string {
	tmp := strings.Split(strings.Trim(string(s.filename[:65]), "\x00"), "\x00")
	return tmp[0]
}

func newBitmapRecord(r io.Reader) (*bitmapRecord, error) {
	record := &bitmapRecord{}
	err := binary.Read(r, binary.LittleEndian, record)
	return record, err
}

// SgBitmap stores references to a series of images
type SgBitmap struct {
	images     []*SgImage
	record     *bitmapRecord
	file       *os.File
	sgFilename string
	bitmapId   int
	isExtern   bool
}

func newSgBitmap(id int, sgFilename string, r io.Reader) (*SgBitmap, error) {
	record, err := newBitmapRecord(r)
	if err != nil {
		return nil, err
	}
	return &SgBitmap{
		bitmapId:   id,
		sgFilename: sgFilename,
		record:     record,
	}, nil
}

// The number of images this bitmap refers
func (sgBitmap *SgBitmap) ImageCount() int {
	return len(sgBitmap.images)
}

// Name of the bitmap along with the number of images
func (sgBitmap *SgBitmap) String() string {
	return fmt.Sprintf("%s (%d)", sgBitmap.record.filenameString(), len(sgBitmap.images))
}

// The name of the bitmap without the extension ".bmp"
func (sgBitmap *SgBitmap) BitmapName() string {
	filename := strings.ToLower(sgBitmap.record.filenameString())
	return strings.Replace(filename, ".bmp", "", -1)
}

// Add an image to the bitmap
func (sgBitmap *SgBitmap) AddImage(child *SgImage) {
	sgBitmap.images = append(sgBitmap.images, child)
}

// Get an image from the bitmap referred by the id
func (sgBitmap *SgBitmap) Image(id int) *SgImage {
	if id < 0 || id >= len(sgBitmap.images) {
		return nil
	}
	return sgBitmap.images[id]
}

// Get an image.RGBA object from the bitmap by the id
func (sgBitmap *SgBitmap) GetImage(id int) (*image.RGBA, error) {
	if id < 0 || id >= len(sgBitmap.images) {
		return nil, errors.New("Id out of bounds")
	}
	return sgBitmap.images[id].GetImage()
}

// Opens the appropriate .555 file to extract data, returns os.File object
func (sgBitmap *SgBitmap) OpenFile(isExtern bool) (*os.File, error) {
	if sgBitmap.file != nil && sgBitmap.isExtern != isExtern {
		sgBitmap.file.Close()
		sgBitmap.file = nil
	}
	sgBitmap.isExtern = isExtern
	if sgBitmap.file == nil {
		filename, err := sgBitmap.find555File()
		if err != nil {
			return nil, err
		}

		file, err := os.Open(filename)
		if err != nil {
			return nil, err
		}
		sgBitmap.file = file
	}
	return sgBitmap.file, nil
}

// Close the .555 file after use
func (sgBitmap *SgBitmap) CloseFile() error {
	return sgBitmap.file.Close()
}

func (sgBitmap *SgBitmap) find555File() (string, error) {
	fi, err := os.Stat(sgBitmap.sgFilename)
	if err != nil {
		return "", err
	}

	// Get the basename of the file
	// either the same name as sg(2|3) or from file record
	basename := ""
	if sgBitmap.isExtern {
		basename = sgBitmap.record.filenameString()
	} else {
		basename = fi.Name()
	}

	// Change the extension to .555
	tmp := strings.SplitAfter(basename, ".")
	if len(tmp) > 1 {
		tmp[len(tmp)-1] = "555"
		basename = strings.Join(tmp, "")
	} else {
		basename += ".555"
	}

	path, err := sgBitmap.findFilenameCaseInsensitive(filepath.Dir(sgBitmap.sgFilename), basename)
	if err == nil {
		return path, nil
	}

	file, err := os.Open(filepath.Dir(sgBitmap.sgFilename) + string(os.PathSeparator) + "555")
	defer file.Close()
	if err != nil {
		return "", err
	}
	path, err = sgBitmap.findFilenameCaseInsensitive(filepath.Dir(sgBitmap.sgFilename)+string(os.PathSeparator)+"555", basename)
	return path, err
}

func (sgBitmap *SgBitmap) findFilenameCaseInsensitive(directory, filename string) (string, error) {
	filename = strings.ToLower(filename)

	dir, err := os.Open(directory)
	defer dir.Close()
	if err != nil {
		return "", err
	}
	files, err := dir.Readdirnames(-1)
	if err != nil {
		return "", err
	}
	for _, file := range files {
		if filename == strings.ToLower(file) {
			return filepath.Abs(directory + string(os.PathSeparator) + file)
		}
	}

	return "", errors.New("File " + filename + " not found in directory " + directory)
}
