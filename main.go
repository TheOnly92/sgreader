package main

import (
	"flag"
    "fmt"
)

func main() {
	// Define the command line flags
	inputFile := flag.String("input", "", "Sets input filename")
	//outputDir := flag.String("output", ".", "Sets directory for output files. Default directory is current")
	//extractSys := flag.Bool("system", false, "Extracts system images from SG2/SG3")
	flag.Parse()

	if *inputFile == "" {
		flag.PrintDefaults()
		return
	}

	sgFile := NewSgFile(*inputFile)
    err := sgFile.Load()
    if err != nil {
        fmt.Println(err)
    }
}
