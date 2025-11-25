package main

import (
	"fmt"
	"os"
	"time"

	"github.com/rwcarlsen/goexif/exif"
	"github.com/rwcarlsen/goexif/mknote"
)

type ExifData struct {
	DateTimeOriginal time.Time
	CameraMake       string
	CameraModel      string
	Orientation      int
	Latitude         float64
	Longitude        float64
	HasLocation      bool
}

func init() {
	// Register manufacturer-specific note parsers so some vendor fields decode correctly.
	exif.RegisterParsers(mknote.All...)
}

// ExtractExif reads common EXIF fields from an image file (pure Go).
// Supports JPEG and TIFF-based formats with EXIF blocks. Returns best-effort data.
func ExtractExif(path string) (*ExifData, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	x, err := exif.Decode(f)
	if err != nil {
		return nil, err
	}

	var out ExifData

	// Date/time
	if tag, err := x.Get(exif.DateTimeOriginal); err == nil {
		if s, err2 := tag.StringVal(); err2 == nil {
			if t, perr := parseExifTime(s); perr == nil {
				out.DateTimeOriginal = t
			}
		}
	} else if t, err := x.DateTime(); err == nil {
		out.DateTimeOriginal = t
	}

	// Make/model
	if tag, err := x.Get(exif.Make); err == nil {
		if s, err2 := tag.StringVal(); err2 == nil {
			out.CameraMake = s
		}
	}
	if tag, err := x.Get(exif.Model); err == nil {
		if s, err2 := tag.StringVal(); err2 == nil {
			out.CameraModel = s
		}
	}

	// Orientation
	if tag, err := x.Get(exif.Orientation); err == nil {
		if i, err2 := tag.Int(0); err2 == nil {
			out.Orientation = i
		}
	}

	// GPS
	if lat, lon, err := x.LatLong(); err == nil {
		out.Latitude = lat
		out.Longitude = lon
		out.HasLocation = true
	}

	return &out, nil
}

func parseExifTime(s string) (time.Time, error) {
	// EXIF time commonly "2006:01:02 15:04:05"
	layouts := []string{
		"2006:01:02 15:04:05",
		time.RFC3339,
	}
	var first error
	for _, l := range layouts {
		if t, err := time.ParseInLocation(l, s, time.Local); err == nil {
			return t, nil
		} else if first == nil {
			first = err
		}
	}
	if first == nil {
		first = fmt.Errorf("unable to parse exif time: %q", s)
	}
	return time.Time{}, first
}


