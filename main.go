package main

import (
	"flag"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	// Define the command line flags
	inputFile := flag.String("input", "", "Sets input filename")
	outputDir := flag.String("output", ".", "Sets directory for output files. Default directory is current")
	extractSys := flag.Bool("system", false, "Extracts system images from SG2/SG3")
	flag.Parse()

	if *inputFile == "" {
		flag.PrintDefaults()
		return
	}

	sgFile := NewSgFile(*inputFile)
	err := sgFile.Load()
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println("Extracting file", *inputFile)

	basename := strings.ToLower(sgFile.Basename())
	// all directories will be lowercase
	workDir, err := filepath.Abs(filepath.Clean(*outputDir + string(os.PathSeparator) + basename))
	if err != nil {
		fmt.Println(err)
		return
	}
	os.Mkdir(workDir, 0755)
	os.Chdir(workDir)

	bitmaps := sgFile.BitmapCount()
	numImages := sgFile.TotalImageCount()
	fmt.Printf("Bitmaps: %d\n", bitmaps)

	i := 0

	if !*extractSys && bitmaps > 1 {
		numImages -= sgFile.ImageCount(0)
		i++
	}

	bmpName := basename
	for i < bitmaps {
		bitmap := sgFile.GetBitmap(i)
		if bitmaps != -1 {
			bmpName = bitmap.BitmapName()
		}
		images := bitmap.ImageCount()

		for n := 0; n < images; n++ {
			img, err := bitmap.GetImage(n)
			if err != nil {
				fmt.Printf("File '%s', image %d: %s\n", basename, n+1, err.Error())
				return
				continue
			}

			pngfilename := strings.ToLower(fmt.Sprintf("%s_%05d.png", bmpName, n+1))
            if _, err := os.Stat(pngfilename); os.IsExist(err) {
                continue
            }
			pngfile, err := os.Create(workDir + string(os.PathSeparator) + pngfilename)
			if err != nil {
				fmt.Println(err)
				return
				continue
			}
			err = png.Encode(pngfile, img)
			if err != nil {
				fmt.Println(err)
				return
				continue
			}
			pngfile.Close()
		}
		bitmap.CloseFile()
		i++
	}
	os.Chdir("..")
}
