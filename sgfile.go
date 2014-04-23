package sgreader

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	headerSize int = 680
)

type SgHeader struct {
	SgFilesize                    uint32
	Version                       uint32
	Unknown1                      uint32
	MaxImageRecords               int32
	NumImageRecords               int32
	NumBitmapRecords              int32
	NumBitmapRecordsWithoutSystem int32
	TotalFilesize                 uint32
	Filesize555                   uint32
	FilesizeExternal              uint32
}

func newHeader(r io.ReadSeeker) (*SgHeader, error) {
	header := &SgHeader{}
	err := binary.Read(r, binary.LittleEndian, header)
	if err != nil {
		return nil, err
	}
	_, err = r.Seek(int64(headerSize), 0)
	return header, err
}

// SgFile holds data for the bitmaps and images stored in the data file
type SgFile struct {
	bitmaps      []*SgBitmap
	images       []*SgImage
	filename     string
	baseFilename string
	header       *SgHeader
}

// Returns a new SgFile object that is tied to the file
func ReadFile(filename string) *SgFile {
	baseFilename := filepath.Base(filename)
	return &SgFile{
		filename:     filename,
		baseFilename: baseFilename,
	}
}

// Attempts to load the bitmaps and images stored within the sg data file
func (sgFile *SgFile) Load() error {
	file, err := os.OpenFile(sgFile.filename, os.O_RDONLY, 0)
	defer file.Close()
	if err != nil {
		return err
	}

	sgFile.header, err = newHeader(file)
	if err != nil {
		return err
	}

	if !sgFile.checkVersion() {
		return errors.New("Incorrect sg version")
	}

	fmt.Printf("Read header, num bitmaps = %d, num images = %d\n", sgFile.header.NumBitmapRecords, sgFile.header.NumImageRecords)

	err = sgFile.loadBitmaps(file)
	if err != nil {
		return err
	}

	_, err = file.Seek(int64(headerSize+sgFile.MaxBitmapRecords()*recordSize), 0)
	if err != nil {
		return err
	}

	err = sgFile.loadImages(file, sgFile.header.Version >= 0xd6)
	if err != nil {
		return err
	}

	if len(sgFile.bitmaps) > 1 && len(sgFile.images) == sgFile.bitmaps[0].ImageCount() {
		fmt.Printf("SG file has %d bitmaps but only the first is in use", len(sgFile.bitmaps))
		// Remove the bitmaps other than the first
		sgFile.bitmaps = sgFile.bitmaps[:0]
	}

	fmt.Printf("Number of images: %d\n", len(sgFile.images))

	return nil
}

func (sgFile *SgFile) loadBitmaps(r io.Reader) error {
	for i := 0; i < int(sgFile.header.NumBitmapRecords); i++ {
		bitmap, err := newSgBitmap(i, sgFile.filename, r)
		if err != nil {
			return err
		}
		sgFile.bitmaps = append(sgFile.bitmaps, bitmap)
	}
	return nil
}

func (sgFile *SgFile) loadImages(r io.Reader, includeAlpha bool) error {
	newSgImage(0, r, includeAlpha)

	for i := 0; i < int(sgFile.header.NumImageRecords); i++ {
		image, err := newSgImage(i+1, r, includeAlpha)
		if err != nil {
			return err
		}
		invertOffset := image.InvertOffset()
		if invertOffset < 0 && (i+int(invertOffset)) >= 0 {
			image.SetInvertImage(sgFile.images[i+int(invertOffset)])
		}
		bitmapId := image.BitmapId()
		if bitmapId >= 0 && bitmapId < len(sgFile.bitmaps) {
			sgFile.bitmaps[bitmapId].AddImage(image)
			image.SetParent(sgFile.bitmaps[bitmapId])
		} else {
			fmt.Printf("Image %d has no parent: %d", i, bitmapId)
		}
		sgFile.images = append(sgFile.images, image)
	}
	return nil
}

func (sgFile *SgFile) checkVersion() bool {
	if sgFile.header.Version == 0xd3 {
		// SG2 file: filesize = 74480 or 522680 (depending on whether it's
		// a "normal" sg2 or an enemy sg2
		if sgFile.header.SgFilesize == 74480 || sgFile.header.SgFilesize == 522680 {
			return true
		}
	} else if sgFile.header.Version == 0xd5 || sgFile.header.Version == 0xd6 {
		fi, err := os.Stat(sgFile.filename)
		if err != nil {
			return false
		}
		if sgFile.header.SgFilesize == 74480 || int64(sgFile.header.SgFilesize) == fi.Size() {
			return true
		}
	}
	return false
}

// Get the maximum number of bitmap records for this sg file
func (sgFile *SgFile) MaxBitmapRecords() int {
	if sgFile.header.Version == 0xd3 {
		return 100 // SG2
	}
	return 200 // SG3
}

// Get the number of images stored within a specific bitmap
func (sgFile *SgFile) ImageCount(bitmapId int) int {
	if bitmapId < 0 || bitmapId >= len(sgFile.bitmaps) {
		return -1
	}

	return sgFile.bitmaps[bitmapId].ImageCount()
}

// Get the bitmap object within the data file
func (sgFile *SgFile) GetBitmap(bitmapId int) *SgBitmap {
	if bitmapId < 0 || bitmapId >= len(sgFile.bitmaps) {
		return nil
	}

	return sgFile.bitmaps[bitmapId]
}

// Get the name of the bitmap and the number of images
func (sgFile *SgFile) GetBitmapDescription(bitmapId int) string {
	if bitmapId < 0 || bitmapId >= len(sgFile.bitmaps) {
		return ""
	}

	return sgFile.bitmaps[bitmapId].String()
}

// Get the basename of the file
func (sgFile *SgFile) Basename() string {
	return sgFile.baseFilename
}

// Get the number of bitmaps stored in the file
func (sgFile *SgFile) BitmapCount() int {
	return len(sgFile.bitmaps)
}

// Get the number of images stored in the file
func (sgFile *SgFile) TotalImageCount() int {
	return len(sgFile.images)
}
