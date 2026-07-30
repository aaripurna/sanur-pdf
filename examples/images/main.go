// Command images demonstrates every way to get a picture into a document, and
// how the four fitting modes behave.
//
// Loading is deliberately separate from layout. The fluent API cannot fail — no
// method on Container returns an error — so reading a file has to happen before
// the layout is described, where the error can be handled properly. The pattern
// throughout is: load once into a core.Image, then hand that value to as many
// .Image() calls as you like.
package main

import (
	"embed"
	"fmt"
	"image"
	"image/color"
	"log"
	"os"
	"path/filepath"

	"codeberg.org/aaripurna/sanur"
	"codeberg.org/aaripurna/sanur/core"
	"codeberg.org/aaripurna/sanur/elements"
	"codeberg.org/aaripurna/sanur/render"
)

// Embedding turns an asset into part of the binary, so a deployed tool needs no
// files alongside it. This is usually what you want for fixed artwork like a
// logo; a user-supplied photograph is better read from disk at runtime.
//
//go:embed assets/logo.png
var logoBytes []byte

// An embed.FS is the other half of the same idea, and is what render.LoadImageFS
// consumes. It is worth having when several assets travel together.
//
//go:embed assets
var assets embed.FS

func main() {
	out := "images.pdf"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}

	sources, err := loadImages()
	if err != nil {
		log.Fatalf("loading images: %v", err)
	}

	doc := sanur.New().
		Title("Image handling").
		Author("sanur").
		Creator("sanur/examples/images")

	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(sanur.Mm(16))
		p.DefaultTextStyle(sanur.TextStyle().Size(9.5).Color(sanur.Grey800))

		// The same logo value is used in the header of every sheet. Sanur pools
		// image resources by key, so the bytes are embedded once no matter how
		// many times they are drawn.
		p.Header().PaddingBottom(12).Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(10)
			c.Item().Row(func(r *sanur.RowBuilder) {
				r.Spacing(10)
				r.ConstantItem(28).AlignMiddle().Image(sources.logo)
				r.RelativeItem(1).AlignMiddle().Column(func(inner *sanur.ColumnBuilder) {
					inner.Item().StyledText("Images", sanur.TextStyle().Size(17).Bold())
					inner.Item().StyledText("loading, fitting and pooling",
						sanur.TextStyle().Size(9).Color(sanur.Grey600))
				})
			})
			c.Item().LineHorizontal(1, sanur.Grey300)
		})

		p.Footer().Row(func(r *sanur.RowBuilder) {
			r.RelativeItem(1).StyledText("sanur/examples/images",
				sanur.TextStyle().Size(8).Color(sanur.Grey500))
			r.ConstantItem(90).AlignRight().
				DefaultTextStyle(sanur.TextStyle().Size(8).Color(sanur.Grey500)).
				PageNumber("Page {page} of {total}")
		})

		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(18)

			c.Item().Element(section("1. Where images come from"))

			c.Item().Column(func(inner *sanur.ColumnBuilder) {
				inner.Spacing(8)
				for _, src := range []struct {
					title string
					code  string
					note  string
					image core.Image
				}{
					{
						"From a file path",
						`img, err := render.LoadImageFile("photo", "assets/photo.jpg")`,
						"Reads the file and detects the format. An empty key defaults to the path.",
						sources.fromPath,
					},
					{
						"From bytes",
						`img, err := render.DecodeImage("logo", logoBytes)`,
						"For data you already hold: an //go:embed asset, an HTTP body, a database blob.",
						sources.fromBytes,
					},
					{
						"From an embedded filesystem",
						`img, err := render.LoadImageFS(assets, "photo-fs", "assets/photo.jpg")`,
						"Takes any fs.FS, so an embed.FS of assets works directly.",
						sources.fromFS,
					},
					{
						"From an image.Image in memory",
						`img, err := render.EncodeJPEG("chart", rendered, 85)`,
						"For pictures you generate: a chart, a QR code, a resized thumbnail.",
						sources.generated,
					},
				} {
					inner.Item().Element(sourceRow(src.title, src.code, src.note, src.image))
				}
			})

			c.Item().Element(section("2. Fitting modes"))

			c.Item().Text("Each panel below is the same 640x400 photograph in an " +
				"identical box 90 points tall. Only the fitting mode differs.")

			// Two modes produce an image taller than the panel: FitWidth scales to
			// the width and overflows downwards, and FitUnscaled ignores the box
			// entirely. Those two are wrapped in Clip so the excess is cropped
			// instead of failing the layout. The other two fit by construction and
			// are left unclipped, because clipping measures its child against
			// unbounded space — which would make FitArea behave exactly like
			// FitWidth and hide the difference this grid exists to show.
			modes := []struct {
				name string
				note string
				fit  elements.ImageFit
				clip bool
			}{
				{"FitWidth", "fills the width; height follows the aspect ratio", elements.FitWidth, true},
				{"FitArea", "fits entirely inside, leaving space on one axis", elements.FitArea, false},
				{"FitStretch", "fills the box exactly, distorting if it must", elements.FitStretch, false},
				{"FitUnscaled", "one pixel per point, cropped to the box", elements.FitUnscaled, true},
			}

			for pair := 0; pair < len(modes); pair += 2 {
				pair := pair
				c.Item().Row(func(r *sanur.RowBuilder) {
					r.Spacing(12)
					for _, mode := range modes[pair : pair+2] {
						r.RelativeItem(1).Element(
							fitPanel(mode.name, mode.note, sources.fromPath, mode.fit, mode.clip))
					}
				})
			}
		})
	})

	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(sanur.Mm(16))
		p.DefaultTextStyle(sanur.TextStyle().Size(9.5).Color(sanur.Grey800))

		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(18)

			c.Item().Element(section("3. Images inside layout"))

			c.Item().Text("An image is an ordinary element, so it composes with " +
				"everything else: rows, table cells, backgrounds, clipping and alignment.")

			// A thumbnail beside text is the most common arrangement in a report.
			c.Item().Background(sanur.Grey100).Padding(14).Row(func(r *sanur.RowBuilder) {
				r.Spacing(14)
				r.ConstantItem(140).Element(&elements.Image{
					Source: sources.fromPath,
					Fit:    elements.FitWidth,
				})
				r.RelativeItem(1).Column(func(inner *sanur.ColumnBuilder) {
					inner.Spacing(5)
					inner.Item().StyledText("Thumbnail beside text",
						sanur.TextStyle().Size(12).Bold())
					inner.Item().RichText(func(tb *sanur.TextBuilder) {
						tb.Align(sanur.AlignJustify)
						tb.Span("A constant-width row item holds the picture at a fixed size " +
							"while a relative item takes the remaining width for the prose. " +
							"Because the row is as tall as its tallest cell, the text block " +
							"and the image stay aligned however long the copy runs.")
					})
				})
			})

			// Clipping is how you crop: measurement is untouched, so the image
			// keeps its aspect ratio and the box decides what is visible.
			c.Item().Column(func(inner *sanur.ColumnBuilder) {
				inner.Spacing(6)
				inner.Item().StyledText("Cropping with Clip", sanur.TextStyle().Size(12).Bold())
				inner.Item().StyledText(
					"An image normally refuses a box it cannot fit, since rendering "+
						"two thirds of a photograph makes no sense. Clip measures its "+
						"child against unbounded space instead, so the image takes its "+
						"natural size and the excess is trimmed.",
					sanur.TextStyle().Size(9).Color(sanur.Grey600))
				inner.Item().Row(func(row *sanur.RowBuilder) {
					row.Spacing(12)
					row.ConstantItem(160).Border(1, sanur.Grey400).Size(160, 60).Clip().
						Element(&elements.Image{Source: sources.fromPath, Fit: elements.FitWidth})
					row.RelativeItem(1).AlignMiddle().StyledText(
						"A 160x60 box clipping an image that would be 160x100 unclipped.",
						sanur.TextStyle().Size(9).Color(sanur.Grey600))
				})
			})

			c.Item().Element(section("4. Logos in a table"))

			c.Item().Table(func(tb *sanur.TableBuilder) {
				tb.ColumnsConstant(34).ColumnRelative(3).ColumnRelative(2)
				tb.ColumnSpacing(10).RowSpacing(2)

				tb.Row(func(tr *sanur.TableRowBuilder) {
					head := sanur.TextStyle().Size(8).Bold().Color(sanur.White).LetterSpacing(0.6)
					tr.Cell().Background(sanur.Indigo).PaddingXY(6, 6).StyledText("", head)
					tr.Cell().Background(sanur.Indigo).PaddingXY(6, 6).StyledText("PRODUCT", head)
					tr.Cell().Background(sanur.Indigo).PaddingXY(6, 6).AlignRight().StyledText("STATUS", head)
				})

				for i, row := range []struct{ name, status string }{
					{"Layout engine", "Shipping"},
					{"Font embedding", "Shipping"},
					{"Image pooling", "Shipping"},
					{"Font subsetting", "Planned"},
				} {
					band := sanur.White
					if i%2 == 1 {
						band = sanur.Grey100
					}
					tb.Row(func(tr *sanur.TableRowBuilder) {
						// The pooled logo reused in every row still costs its
						// bytes only once in the finished file.
						tr.Cell().Background(band).PaddingXY(6, 5).AlignMiddle().Image(sources.logo)
						tr.Cell().Background(band).PaddingXY(6, 9).Text(row.name)
						tr.Cell().Background(band).PaddingXY(6, 9).AlignRight().Text(row.status)
					})
				}
			})

			c.Item().Background(sanur.Hex("#FFF8E1")).BorderLeft(3, sanur.Orange).Padding(12).
				RichText(func(tb *sanur.TextBuilder) {
					tb.Bold("Transparency. ")
					tb.Span("The logo is a PNG with an alpha channel. PDF image samples " +
						"carry no alpha, so sanur emits the transparency as a separate " +
						"greyscale soft mask. JPEGs are embedded byte-for-byte instead, " +
						"since PDF speaks their codec natively and re-encoding would only " +
						"lose quality.")
				})
		})
	})

	if err := doc.Write(out); err != nil {
		log.Fatalf("generating document: %v", err)
	}
	fmt.Printf("wrote %s\n", out)
}

// imageSet holds one image per loading route.
type imageSet struct {
	logo      core.Image
	fromPath  core.Image
	fromBytes core.Image
	fromFS    core.Image
	generated core.Image
}

func loadImages() (imageSet, error) {
	var set imageSet
	var err error

	// From bytes already in memory. An //go:embed variable is the common case,
	// but an HTTP response body or a database blob works identically.
	set.fromBytes, err = render.DecodeImage("logo", logoBytes)
	if err != nil {
		return set, err
	}
	set.logo = set.fromBytes

	// From a path on disk. Resolved relative to this source file so the example
	// runs from any working directory.
	set.fromPath, err = render.LoadImageFile("photo", filepath.Join(assetDir(), "photo.jpg"))
	if err != nil {
		return set, err
	}

	// From any fs.FS, including the embed.FS declared above.
	set.fromFS, err = render.LoadImageFS(assets, "photo-fs", "assets/photo.jpg")
	if err != nil {
		return set, err
	}

	// From an image.Image built at runtime. EncodeJPEG is the bridge for anything
	// you draw yourself; use a PNG encoder instead when the picture needs alpha
	// or has hard edges.
	set.generated, err = render.EncodeJPEG("generated", swatch(), 85)
	if err != nil {
		return set, err
	}

	return set, nil
}

// assetDir locates the assets directory next to this program, falling back to a
// relative path when the executable was built rather than run from source.
func assetDir() string {
	if wd, err := os.Getwd(); err == nil {
		candidate := filepath.Join(wd, "examples", "images", "assets")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "assets"
}

// swatch builds a small gradient in memory, standing in for a generated chart.
func swatch() image.Image {
	const w, h = 320, 200
	img := image.NewRGBA(image.Rect(0, 0, w, h))

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8(60 + 160*x/w),
				G: uint8(90 + 120*y/h),
				B: 190,
				A: 255,
			})
		}
	}
	return img
}

// section renders a heading with a rule beneath it.
func section(title string) core.Element {
	col := &elements.Column{Spacing: 5}
	col.Add(elements.NewText(title, sanur.TextStyle().Size(13).Bold().Color(sanur.Indigo).Build()))
	col.Add(&elements.Line{Width: 0.75, Color: sanur.Grey300})
	return col
}

// sourceRow describes one loading route: a thumbnail, a title, the call, and a note.
func sourceRow(title, code, note string, img core.Image) core.Element {
	thumb := &elements.Constrained{
		MinWidth: 56, MaxWidth: 56,
		Child: &elements.Image{Source: img, Fit: elements.FitWidth},
	}

	text := &elements.Column{Spacing: 3}
	text.Add(elements.NewText(title, sanur.TextStyle().Size(10.5).Bold().Build()))
	text.Add(elements.NewText(code, sanur.TextStyle().Mono().Size(8).Color(sanur.Teal).Build()))
	text.Add(elements.NewText(note, sanur.TextStyle().Size(8.5).Color(sanur.Grey600).Build()))

	row := elements.NewRow(12,
		elements.Constant(56, &elements.Aligned{Vertical: core.AlignMiddle, Child: thumb}),
		elements.Relative(1, &elements.Aligned{Vertical: core.AlignMiddle, Child: text}),
	)

	return &elements.Background{
		Color: sanur.Grey100,
		Child: elements.UniformPadding(10, row),
	}
}

// fitPanel shows one fitting mode in a box 90 points tall, with a caption.
func fitPanel(name, note string, img core.Image, fit elements.ImageFit, clip bool) core.Element {
	var content core.Element = &elements.Image{Source: img, Fit: fit}
	if clip {
		content = &elements.Clip{Child: content}
	}

	box := elements.UniformBorder(1, sanur.Grey400, &elements.Constrained{
		MinHeight: 90, MaxHeight: 90,
		Child: content,
	})

	col := &elements.Column{Spacing: 4}
	col.Add(elements.NewText(name, sanur.TextStyle().Mono().Size(9).Bold().Build()))
	col.Add(box)
	col.Add(elements.NewText(note, sanur.TextStyle().Size(8).Color(sanur.Grey600).Build()))
	return col
}
