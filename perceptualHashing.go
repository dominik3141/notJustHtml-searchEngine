package main

import (
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"

	"github.com/corona10/goimagehash"
	"github.com/rwcarlsen/goexif/exif"
)

func calcPercptualHashes(mimeType string, file io.Reader, urlStr string) *PerceptualHash {
	var err error
	var img image.Image

	switch mimeType {
	case "image/jpeg":
		img, err = jpeg.Decode(file)
	case "image/png":
		img, err = png.Decode(file)
	default:
		err = errors.New("unknown mime-type")
	}
	if err != nil {
		logErrorToDb(err, ErrorPerceptualHash, urlStr)
		return nil
	}

	hashes := new(PerceptualHash)

	hash, err := goimagehash.AverageHash(img)
	if err != nil {
		logErrorToDb(err, ErrorPerceptualHash, urlStr)
	}
	hashes.AverageHash = hash.GetHash()

	hash, err = goimagehash.DifferenceHash(img)
	if err != nil {
		logErrorToDb(err, ErrorPerceptualHash, urlStr)
	}
	hashes.DifferenceHash = hash.GetHash()

	hash, err = goimagehash.PerceptionHash(img)
	if err != nil {
		logErrorToDb(err, ErrorPerceptualHash, urlStr)
	}
	hashes.PerceptionHash = hash.GetHash()

	return hashes
}

func getExif(file io.Reader, urlStr string) *ExifInfo {
	x, err := exif.Decode(file)
	if err != nil {
		return nil
	}

	ret := new(ExifInfo)

	camModel, err := x.Get(exif.Model)
	if err == nil {
		camModelStr, err := camModel.StringVal()
		if err == nil {
			ret.Camera = camModelStr
		}
	}

	tm, err := x.DateTime()
	if err == nil {
		ret.Timestamp = tm.UnixMicro()
	}

	lat, long, err := x.LatLong()
	if err == nil {
		ret.Lat, ret.Long = lat, long
	}

	if ret.Timestamp == 0 && ret.Camera == "" && ret.Lat == 0 {
		return nil
	} else {
		return ret
	}
}
