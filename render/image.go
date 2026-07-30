package render

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // registers the PNG decoder used by DecodeImage
	"io/fs"
	"os"

	"github.com/aaripurna/sanur-pdf/core"
	"github.com/aaripurna/sanur-pdf/internal/pdfobj"
)

// DecodeImage inspects encoded image bytes and prepares them for embedding.
//
// Only the dimensions and format are read here; the bytes are kept as supplied.
// That matters for JPEG, which PDF can embed verbatim — decoding and re-encoding
// it would throw away quality and inflate the file for nothing.
func DecodeImage(key string, data []byte) (core.Image, error) {
	if len(data) == 0 {
		return core.Image{}, fmt.Errorf("sanur/render: image %q is empty", key)
	}

	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return core.Image{}, fmt.Errorf("sanur/render: decoding image %q: %w", key, err)
	}
	switch format {
	case "jpeg", "png":
	default:
		return core.Image{}, fmt.Errorf(
			"sanur/render: image %q has unsupported format %q (want jpeg or png)", key, format)
	}

	return core.Image{
		Key:         key,
		Format:      format,
		Data:        data,
		PixelWidth:  cfg.Width,
		PixelHeight: cfg.Height,
	}, nil
}

// LoadImageFile reads and prepares an image from disk, naming it after the file
// if key is empty.
//
// The key is what deduplicates the image across the document, so it matters that
// two loads of the same picture agree on it. Defaulting to the path means the
// obvious usage — loading a logo once per page — costs its bytes once.
func LoadImageFile(key, path string) (core.Image, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return core.Image{}, fmt.Errorf("sanur/render: reading image %s: %w", path, err)
	}
	if key == "" {
		key = path
	}
	return DecodeImage(key, data)
}

// LoadImageFS reads an image from a filesystem, which is how an embedded asset
// declared with //go:embed is loaded.
func LoadImageFS(fsys fs.FS, key, name string) (core.Image, error) {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return core.Image{}, fmt.Errorf("sanur/render: reading image %s: %w", name, err)
	}
	if key == "" {
		key = name
	}
	return DecodeImage(key, data)
}

// imageResource registers an image and returns its XObject resource name.
func (b *Builder) imageResource(img core.Image) (string, error) {
	if name, ok := b.imageNames[img.Key]; ok {
		return name, nil
	}

	name := fmt.Sprintf("Im%d", len(b.imageOrder))
	ref, err := b.emitImage(img)
	if err != nil {
		return "", err
	}

	b.imageNames[img.Key] = name
	b.imageRefs[name] = ref
	b.imageOrder = append(b.imageOrder, name)
	return name, nil
}

func (b *Builder) emitImage(img core.Image) (pdfobj.Ref, error) {
	if img.Format == "jpeg" {
		// DCTDecode is the JPEG codec built into PDF, so the original file is
		// passed through untouched — which means its colour space has to be
		// declared from outside the data, read from the frame header rather than
		// assumed. A greyscale or CMYK JPEG labelled DeviceRGB is rejected by
		// readers and, where tolerated, renders in the wrong colours.
		info, err := scanJPEG(img.Data)
		if err != nil {
			return 0, fmt.Errorf("sanur/render: image %q: %w", img.Key, err)
		}

		dict, err := jpegDict(info, img.PixelWidth, img.PixelHeight)
		if err != nil {
			return 0, fmt.Errorf("sanur/render: image %q: %w", img.Key, err)
		}
		return b.writer.AddStream(dict, img.Data), nil
	}
	return b.emitRaster(img)
}

// emitRaster decodes an image and writes it as uncompressed samples, letting the
// writer's Flate filter compress them.
//
// PNG's own compression cannot be reused directly: PDF supports the same Flate
// filter and predictors, but only for images whose colour type, bit depth and
// interlacing all line up with what a PDF image expects. Decoding to RGB
// sidesteps that entirely, and Flate on the raw samples lands close to the
// original size for the flat graphics PNG is normally used for.
func (b *Builder) emitRaster(img core.Image) (pdfobj.Ref, error) {
	decoded, _, err := image.Decode(bytes.NewReader(img.Data))
	if err != nil {
		return 0, fmt.Errorf("sanur/render: decoding image %q: %w", img.Key, err)
	}

	bounds := decoded.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	rgb := make([]byte, 0, w*h*3)
	alpha := make([]byte, 0, w*h)
	transparent := false

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			// RGBA returns 16-bit alpha-premultiplied values; PDF wants 8-bit
			// straight colour, so each channel is un-premultiplied before being
			// narrowed. A fully transparent pixel has no recoverable colour, so
			// it is written as black.
			r16, g16, b16, a16 := decoded.At(x, y).RGBA()
			if a16 == 0 {
				rgb = append(rgb, 0, 0, 0)
				alpha = append(alpha, 0)
				transparent = true
				continue
			}
			rgb = append(rgb,
				byte(r16*0xFFFF/a16>>8),
				byte(g16*0xFFFF/a16>>8),
				byte(b16*0xFFFF/a16>>8),
			)
			a := byte(a16 >> 8)
			alpha = append(alpha, a)
			if a != 0xFF {
				transparent = true
			}
		}
	}

	dict := imageDict(w, h, "DeviceRGB", 8)

	if transparent {
		// Transparency travels in a separate greyscale soft mask, since a PDF
		// image's colour samples have no alpha channel of their own.
		maskDict := imageDict(w, h, "DeviceGray", 8)
		dict.SetRef("SMask", b.writer.AddStream(maskDict, alpha))
	}

	return b.writer.AddStream(dict, rgb), nil
}

func imageDict(w, h int, colorSpace string, bpc int) *pdfobj.Dict {
	return pdfobj.NewDict().
		SetName("Type", "XObject").
		SetName("Subtype", "Image").
		SetInt("Width", w).
		SetInt("Height", h).
		SetName("ColorSpace", colorSpace).
		SetInt("BitsPerComponent", bpc)
}

// EncodeJPEG is a convenience for callers holding a decoded image rather than an
// encoded file.
func EncodeJPEG(key string, img image.Image, quality int) (core.Image, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return core.Image{}, fmt.Errorf("sanur/render: encoding image %q: %w", key, err)
	}
	return DecodeImage(key, buf.Bytes())
}
