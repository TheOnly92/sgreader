package main

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
	RECORD_SIZE int = 200
)

type SgBitmapRecord struct {
	Filename   [65]byte
	Comment    [51]byte
	Width      uint32
	Height     uint32
	NumImages  uint32
	StartIndex uint32
	EndIndex   uint32
	_          [64]byte
}

func (s *SgBitmapRecord) FilenameString() string {
	tmp := strings.Split(strings.Trim(string(s.Filename[:65]), "\x00"), "\x00")
	return tmp[0]
}

func NewSgBitmapRecord(r io.Reader) (*SgBitmapRecord, error) {
	record := &SgBitmapRecord{}
	err := binary.Read(r, binary.LittleEndian, record)
	return record, err
}

type SgBitmap struct {
	images     []*SgImage
	record     *SgBitmapRecord
	file       *os.File
	sgFilename string
	bitmapId   int
	isExtern   bool
}

func NewSgBitmap(id int, sgFilename string, r io.Reader) (*SgBitmap, error) {
	record, err := NewSgBitmapRecord(r)
	if err != nil {
		return nil, err
	}
	return &SgBitmap{
		bitmapId:   id,
		sgFilename: sgFilename,
		record:     record,
	}, nil
}

func (sgBitmap *SgBitmap) ImageCount() int {
	return len(sgBitmap.images)
}

func (sgBitmap *SgBitmap) String() string {
	return fmt.Sprintf("%s (%d)", sgBitmap.record.Filename, len(sgBitmap.images))
}

func (sgBitmap *SgBitmap) BitmapName() string {
	filename := strings.ToLower(sgBitmap.record.FilenameString())
	return strings.Replace(filename, ".bmp", "", -1)
}

func (sgBitmap *SgBitmap) AddImage(child *SgImage) {
	sgBitmap.images = append(sgBitmap.images, child)
}

func (sgBitmap *SgBitmap) Image(id int) *SgImage {
	if id < 0 || id >= len(sgBitmap.images) {
		return nil
	}
	return sgBitmap.images[id]
}

func (sgBitmap *SgBitmap) GetImage(id int) (*image.RGBA, error) {
	if id < 0 || id >= len(sgBitmap.images) {
		return nil, errors.New("Id out of bounds")
	}
	return sgBitmap.images[id].GetImage()
}

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
		basename = sgBitmap.record.FilenameString()
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
