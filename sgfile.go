package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	SG_HEADER_SIZE int = 680
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

func NewSgHeader(r io.Reader) (*SgHeader, error) {
	header := &SgHeader{}
	err := binary.Read(r, binary.LittleEndian, header)
	return header, err
}

type SgFile struct {
	bitmaps      []*SgBitmap
	images       []*SgImage
	filename     string
	baseFilename string
	header       *SgHeader
}

func NewSgFile(filename string) *SgFile {
	baseFilename := filepath.Base(filename)
	return &SgFile{
		filename:     filename,
		baseFilename: baseFilename,
	}
}

func (sgFile *SgFile) Load() error {
	file, err := os.OpenFile(sgFile.filename, os.O_RDONLY, 0)
	if err != nil {
		return err
	}

	sgFile.header, err = NewSgHeader(file)
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

	_, err = file.Seek(int64(SG_HEADER_SIZE+sgFile.MaxBitmapRecords()*RECORD_SIZE), 0)
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
		bitmap, err := NewSgBitmap(i, sgFile.filename, r)
		if err != nil {
			return err
		}
		sgFile.bitmaps = append(sgFile.bitmaps, bitmap)
	}
	return nil
}

func (sgFile *SgFile) loadImages(r io.Reader, includeAlpha bool) error {
	NewSgImage(0, r, includeAlpha)

	for i := 0; i < int(sgFile.header.NumImageRecords); i++ {
		image, err := NewSgImage(i+1, r, includeAlpha)
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

func (sgFile *SgFile) MaxBitmapRecords() int {
	if sgFile.header.Version == 0xd3 {
		return 100 // SG2
	}
	return 200 // SG3
}
