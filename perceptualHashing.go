package main

import (
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log"

	"github.com/corona10/goimagehash"
)

func calcAvgHash(mimeType string, file io.Reader) (uint64, error) {
	var err error
	var img image.Image

	switch mimeType {
	case "image/jpeg":
		img, err = jpeg.Decode(file)
	case "image/png":
		img, err = png.Decode(file)
	default:
		return 0, errors.New("Unknown mime-type")
	}
	if err != nil {
		log.Println("ERROR in calcAvgHash:", err)
		return 0, err
	}

	hash, err := goimagehash.AverageHash(img)

	return hash.GetHash(), nil
}
