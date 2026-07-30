package render

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/aaripurna/sanur-pdf/internal/pdfobj"
)

// JPEG marker bytes. Every marker is a 0xFF byte followed by one of these.
const (
	markerSOI   = 0xD8 // start of image
	markerEOI   = 0xD9 // end of image
	markerSOS   = 0xDA // start of scan: entropy-coded data follows
	markerAPP14 = 0xEE // where Adobe records that CMYK values are inverted
	markerTEM   = 0x01
)

// jpegInfo is what the PDF writer needs to know about a JPEG's colour channels.
//
// PDF embeds a JPEG without decoding it, which means the colour space has to be
// declared from outside the data. Get it wrong and a reader either refuses the
// image or renders it in the wrong colours — so the channel count is read from
// the file rather than assumed.
type jpegInfo struct {
	// components is the number of colour channels: 1 for greyscale, 3 for
	// YCbCr, 4 for CMYK or YCCK.
	components int

	// adobe reports an APP14 "Adobe" segment, which marks a file whose CMYK
	// values are stored inverted.
	adobe bool
}

// scanJPEG reads a JPEG's markers far enough to classify its colour channels.
//
// This deliberately does not go through image.DecodeConfig. That reports a
// color.Model rather than a channel count, so the mapping back to a PDF colour
// space would rest on an implementation detail of the standard library's decoder;
// and it needs the entropy-coded scan to be present, which makes a malformed or
// truncated file indistinguishable from an unsupported one. Reading the frame
// header directly is a few lines and answers exactly the question being asked.
func scanJPEG(data []byte) (jpegInfo, error) {
	var info jpegInfo

	if len(data) < 4 || data[0] != 0xFF || data[1] != markerSOI {
		return info, fmt.Errorf("not a JPEG: missing start-of-image marker")
	}

	// Scanning continues past the frame header rather than stopping at it,
	// because the Adobe segment may sit either side of it.
	for i := 2; i+1 < len(data); {
		if data[i] != 0xFF {
			return info, fmt.Errorf("malformed JPEG: expected a marker at byte %d", i)
		}

		marker := data[i+1]

		// Padding between segments is legal and encoded as repeated 0xFF.
		if marker == 0xFF {
			i++
			continue
		}
		// Markers that carry no payload.
		if marker == markerSOI || marker == markerTEM || marker == markerEOI ||
			(marker >= 0xD0 && marker <= 0xD7) {
			i += 2
			continue
		}

		if i+4 > len(data) {
			break
		}
		length := int(binary.BigEndian.Uint16(data[i+2 : i+4]))
		if length < 2 {
			return info, fmt.Errorf("malformed JPEG: segment length %d at byte %d", length, i)
		}

		end := i + 2 + length
		if end > len(data) {
			end = len(data)
		}
		payload := data[i+4 : end]

		switch {
		case marker == markerAPP14:
			if bytes.HasPrefix(payload, []byte("Adobe")) {
				info.adobe = true
			}

		case isFrameHeader(marker):
			// A frame header is precision, height, width, then the channel count.
			if len(payload) < 6 {
				return info, fmt.Errorf("malformed JPEG: truncated frame header")
			}
			info.components = int(payload[5])

		case marker == markerSOS:
			// The scan data begins here; nothing further is a marker segment.
			if info.components == 0 {
				return info, fmt.Errorf("malformed JPEG: no frame header before the scan")
			}
			return info, nil
		}

		i = end
	}

	if info.components == 0 {
		return info, fmt.Errorf("malformed JPEG: no frame header found")
	}
	return info, nil
}

// isFrameHeader reports whether a marker introduces a start-of-frame segment.
//
// The 0xC0..0xCF range is shared: most values are frame headers for the various
// JPEG modes, but three carry unrelated tables and must not be read as one.
func isFrameHeader(marker byte) bool {
	if marker < 0xC0 || marker > 0xCF {
		return false
	}
	switch marker {
	case 0xC4, // define Huffman table
		0xC8, // reserved extension
		0xCC: // define arithmetic coding conditioning
		return false
	}
	return true
}

// jpegDict builds the image dictionary for a JPEG embedded verbatim.
func jpegDict(info jpegInfo, width, height int) (*pdfobj.Dict, error) {
	var space string
	switch info.components {
	case 1:
		space = "DeviceGray"
	case 3:
		space = "DeviceRGB"
	case 4:
		space = "DeviceCMYK"
	default:
		return nil, fmt.Errorf(
			"unsupported JPEG with %d colour channels (want 1, 3 or 4)", info.components)
	}

	dict := imageDict(width, height, space, 8).SetName("Filter", "DCTDecode")

	// Adobe writes CMYK JPEGs with every channel inverted, and records the fact
	// only by its presence in an APP14 segment. PDF has no equivalent flag, so the
	// inversion is undone with a Decode array instead; without it the image
	// renders as a photographic negative.
	if info.components == 4 && info.adobe {
		dict.Set("Decode", pdfobj.NumArray(1, 0, 1, 0, 1, 0, 1, 0))
	}

	return dict, nil
}
