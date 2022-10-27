package main

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"io"

	"github.com/Kagami/go-face"
)

type Face struct {
	ID         int64 `bun:",pk,autoincrement"`
	ContentId  int64
	Descriptor face.Descriptor `bun:",array"`
	Rectangle  image.Rectangle
	Shapes     []image.Point
}

func indexFile(fileBytes *[]byte, useCNN bool, isPNG bool) ([]face.Face, error) {
	const modelsDir = "faceRecognition/models"

	// should we rather reuse a single face recognizer?
	rec, err := face.NewRecognizer(modelsDir)
	check(err)

	var faces []face.Face

	if isPNG {
		reader := bytes.NewReader(*fileBytes)

		// convert to jpeg
		img, err := png.Decode(reader)
		if err != nil {
			return nil, err
		}

		buf := new(bytes.Buffer)

		err = jpeg.Encode(buf, img, nil)
		check(err)

		jpgBytes, err := io.ReadAll(buf)
		check(err)

		fileBytes = &jpgBytes
	}

	if useCNN {
		faces, err = rec.RecognizeCNN(*fileBytes)
	} else {
		faces, err = rec.Recognize(*fileBytes)
	}
	if err != nil {
		return nil, err
	}

	return faces, nil
}
