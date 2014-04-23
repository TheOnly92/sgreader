# sgreader

## Installation

    go get github.com/TheOnly92/sgreader

## Documentation

<http://go.pkgdoc.org/github.com/TheOnly92/sgreader>

## Usage

    package main

    import (
        "github.com/TheOnly92/sgreader"
    )

    func main() {
        file := sgreader.ReadFile("C3.sg2")
        err := file.Load()
        if err != nil {
            panic(err)
        }

        bitmaps := file.BitmapCount()
        fmt.Printf("Bitmaps: %d\n", bitmaps)
    }
