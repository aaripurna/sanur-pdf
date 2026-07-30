package render

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

// The header scanner is tested from inside the package because that is the only
// way to reach the CMYK path: Go's standard library cannot encode a four-channel
// JPEG, so no real fixture can be produced here. Synthesising a header is enough,
// since the scanner reads nothing beyond the marker segments.

// jpegHeader builds a JPEG marker sequence with the given channel count.
type jpegHeader struct {
	components    int
	width, height int
	adobe         bool
	adobeAfterSOF bool
	withScan      bool
	frameMarker   byte
}

func (h jpegHeader) build() []byte {
	marker := h.frameMarker
	if marker == 0 {
		marker = 0xC0 // baseline
	}

	var b bytes.Buffer
	b.Write([]byte{0xFF, markerSOI})

	writeAdobe := func() {
		payload := []byte("Adobe\x00d\x00\x00\x00\x00\x00")
		b.Write([]byte{0xFF, markerAPP14})
		binary.Write(&b, binary.BigEndian, uint16(len(payload)+2))
		b.Write(payload)
	}

	if h.adobe && !h.adobeAfterSOF {
		writeAdobe()
	}

	var frame bytes.Buffer
	frame.WriteByte(8) // sample precision
	binary.Write(&frame, binary.BigEndian, uint16(h.height))
	binary.Write(&frame, binary.BigEndian, uint16(h.width))
	frame.WriteByte(byte(h.components))
	for i := 1; i <= h.components; i++ {
		frame.WriteByte(byte(i)) // component id
		frame.WriteByte(0x11)    // 1x1 sampling
		frame.WriteByte(0)       // quantisation table
	}
	b.Write([]byte{0xFF, marker})
	binary.Write(&b, binary.BigEndian, uint16(frame.Len()+2))
	b.Write(frame.Bytes())

	if h.adobe && h.adobeAfterSOF {
		writeAdobe()
	}

	if h.withScan {
		b.Write([]byte{0xFF, markerSOS, 0x00, 0x02})
		b.Write([]byte{0x01, 0x02, 0x03}) // stand-in for entropy-coded data
	}

	return b.Bytes()
}

func TestScanJPEGReadsChannelCount(t *testing.T) {
	for _, components := range []int{1, 3, 4} {
		info, err := scanJPEG(jpegHeader{components: components, width: 40, height: 20}.build())
		if err != nil {
			t.Errorf("%d channels: %v", components, err)
			continue
		}
		if info.components != components {
			t.Errorf("got %d channels, want %d", info.components, components)
		}
	}
}

func TestScanJPEGDetectsTheAdobeMarker(t *testing.T) {
	plain, err := scanJPEG(jpegHeader{components: 4, width: 8, height: 8}.build())
	if err != nil {
		t.Fatal(err)
	}
	if plain.adobe {
		t.Error("reported an Adobe marker that is not present")
	}

	// The segment conventionally precedes the frame header, but nothing requires
	// it to, so both orders have to be recognised.
	for _, after := range []bool{false, true} {
		info, err := scanJPEG(jpegHeader{
			components: 4, width: 8, height: 8,
			adobe: true, adobeAfterSOF: after, withScan: true,
		}.build())
		if err != nil {
			t.Errorf("adobeAfterSOF=%v: %v", after, err)
			continue
		}
		if !info.adobe {
			t.Errorf("adobeAfterSOF=%v: the Adobe marker was missed", after)
		}
	}
}

func TestScanJPEGHandlesEveryFrameMode(t *testing.T) {
	// Progressive and lossless JPEGs use other markers in the same range, and all
	// of them carry the channel count in the same place.
	for _, marker := range []byte{0xC0, 0xC1, 0xC2, 0xC3, 0xC5, 0xC6, 0xC7, 0xC9, 0xCA, 0xCB, 0xCD, 0xCE, 0xCF} {
		info, err := scanJPEG(jpegHeader{
			components: 3, width: 8, height: 8, frameMarker: marker,
		}.build())
		if err != nil {
			t.Errorf("marker 0x%02X: %v", marker, err)
			continue
		}
		if info.components != 3 {
			t.Errorf("marker 0x%02X: got %d channels, want 3", marker, info.components)
		}
	}
}

func TestScanJPEGIgnoresTableMarkers(t *testing.T) {
	// 0xC4, 0xC8 and 0xCC share the frame-header range but carry tables. Read as a
	// frame header, a Huffman table's bytes would yield a nonsense channel count.
	var b bytes.Buffer
	b.Write([]byte{0xFF, markerSOI})

	// A define-Huffman-table segment long enough to be misread.
	table := make([]byte, 20)
	b.Write([]byte{0xFF, 0xC4})
	binary.Write(&b, binary.BigEndian, uint16(len(table)+2))
	b.Write(table)

	b.Write(jpegHeader{components: 1, width: 8, height: 8}.build()[2:])

	info, err := scanJPEG(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if info.components != 1 {
		t.Errorf("got %d channels, want 1: a table marker was read as a frame header",
			info.components)
	}
}

func TestScanJPEGRejectsMalformedInput(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"empty", nil},
		{"too short", []byte{0xFF, 0xD8}},
		{"not a jpeg", []byte("this is not an image at all")},
		{"no start marker", []byte{0x00, 0x00, 0x00, 0x00}},
		// A file whose scan begins with no frame header has no channel count.
		{"scan without frame", []byte{0xFF, markerSOI, 0xFF, markerSOS, 0x00, 0x02, 0x01}},
	} {
		if _, err := scanJPEG(tc.data); err == nil {
			t.Errorf("%s: expected an error", tc.name)
		}
	}
}

func TestJPEGDictSelectsTheColourSpace(t *testing.T) {
	for _, tc := range []struct {
		components int
		want       string
	}{
		{1, "/ColorSpace /DeviceGray"},
		{3, "/ColorSpace /DeviceRGB"},
		{4, "/ColorSpace /DeviceCMYK"},
	} {
		dict, err := jpegDict(jpegInfo{components: tc.components}, 40, 20)
		if err != nil {
			t.Errorf("%d channels: %v", tc.components, err)
			continue
		}
		got := dict.String()
		if !strings.Contains(got, tc.want) {
			t.Errorf("%d channels: dict %q lacks %q", tc.components, got, tc.want)
		}
		if !strings.Contains(got, "/Filter /DCTDecode") {
			t.Errorf("%d channels: dict %q lacks the JPEG filter", tc.components, got)
		}
	}
}

func TestJPEGDictInvertsAdobeCMYK(t *testing.T) {
	// Adobe stores CMYK inverted and signals it only by the marker's presence.
	// Without a Decode array the image renders as a photographic negative.
	adobe, err := jpegDict(jpegInfo{components: 4, adobe: true}, 8, 8)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(adobe.String(), "/Decode [1 0 1 0 1 0 1 0]") {
		t.Errorf("an Adobe CMYK JPEG needs an inverting Decode array; got %q", adobe.String())
	}

	// A CMYK JPEG from any other producer is stored the normal way round.
	plain, err := jpegDict(jpegInfo{components: 4}, 8, 8)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plain.String(), "/Decode") {
		t.Errorf("a non-Adobe CMYK JPEG should not be inverted; got %q", plain.String())
	}

	// The inversion applies to CMYK only: three channels with an Adobe marker is
	// an ordinary RGB photograph.
	rgb, err := jpegDict(jpegInfo{components: 3, adobe: true}, 8, 8)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rgb.String(), "/Decode") {
		t.Errorf("an RGB JPEG should never be inverted; got %q", rgb.String())
	}
}

func TestJPEGDictRejectsUnsupportedChannelCounts(t *testing.T) {
	for _, components := range []int{0, 2, 5, 9} {
		if _, err := jpegDict(jpegInfo{components: components}, 8, 8); err == nil {
			t.Errorf("%d channels: expected an error", components)
		}
	}
}
