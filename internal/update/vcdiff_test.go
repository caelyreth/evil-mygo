package update

import (
	"bytes"
	"math/rand/v2"
	"slices"
	"testing"
	"time"
)

func TestVCDIFFIntegers(t *testing.T) {
	for _, tc := range []struct {
		v    uint64
		want []byte
	}{
		{0, []byte{0x00}},
		{127, []byte{0x7F}},
		{128, []byte{0x81, 0x00}},
		{16383, []byte{0xFF, 0x7F}},
		{16384, []byte{0x81, 0x80, 0x00}},
	} {
		got := appendVCDInt(nil, tc.v)
		if !bytes.Equal(got, tc.want) {
			t.Errorf("appendVCDInt(%d) = %x, want %x", tc.v, got, tc.want)
		}
		v, err := readVCDInt(bytes.NewReader(got))
		if err != nil || v != tc.v {
			t.Errorf("readVCDInt(%x) = %d, %v", got, v, err)
		}
	}
}

func TestVCDIFFEncodeDecode(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	random := func(n int) []byte {
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(r.Uint32())
		}
		return b
	}
	base := random(200_000)
	edited := slices.Clone(base)
	for range 500 {
		edited[r.IntN(len(edited))] += byte(1 + r.IntN(3))
	}
	edited = slices.Concat(edited[100_000:150_000], random(3000), edited[:100_000], edited[160_000:])
	cases := []struct {
		name     string
		old, new []byte
	}{
		{"empty", nil, nil},
		{"from empty", nil, []byte("hello")},
		{"to empty", []byte("hello"), nil},
		{"same", base, base},
		{"edited", base, edited},
		{"unrelated", random(5000), random(7000)},
		{"zeros", make([]byte, 100_000), append(make([]byte, 50_000), 1)},
	}
	for _, c := range cases {
		patch := EncodeVCDIFF(c.old, c.new)
		var out bytes.Buffer
		if err := DecodeVCDIFF(&out, int64(len(c.new)), bytes.NewReader(c.old), int64(len(c.old)), bytes.NewReader(patch)); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if !bytes.Equal(out.Bytes(), c.new) {
			t.Fatalf("%s: patching made %d bytes that differ from %d", c.name, out.Len(), len(c.new))
		}
		if c.name == "edited" && len(patch) > len(c.new)/10 {
			t.Errorf("edited VCDIFF patch takes %d bytes, for %d", len(patch), len(c.new))
		}
	}
}

func TestVCDIFFShiftedMatch(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 6))
	x := make([]byte, 1<<20)
	for i := range x {
		x[i] = byte(r.Uint32())
	}
	near := slices.Clone(x)
	near[0]++
	near[len(near)-1]++
	head := []byte("a header found once in the old file")
	old, new := slices.Concat(x, head, near), slices.Concat(head, x)
	done := make(chan []byte, 1)
	go func() { done <- EncodeVCDIFF(old, new) }()
	var patch []byte
	select {
	case patch = <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("EncodeVCDIFF takes too long")
	}
	var out bytes.Buffer
	if err := DecodeVCDIFF(&out, int64(len(new)), bytes.NewReader(old), int64(len(old)), bytes.NewReader(patch)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out.Bytes(), new) {
		t.Fatal("patching made bytes that differ from new")
	}
}

func TestVCDIFFStandardRawVector(t *testing.T) {
	// Hand-assembled RFC 3284 stream from RFC 3284 Section 7 / manual VCDIFF
	// proving standard raw VCDIFF compatibility (header + VCD_SOURCE + COPY + ADD + VCD_TARGET).
	old := []byte("abcdef")
	// Encode source=abcd + RUN('z'*4) + target-copy("bcdz")
	// u = "abcdef" (len 6)
	// Window 1: VCD_SOURCE (len=6, pos=0), targetLen=8, inst: COPY(4) from addr 0 (VCD_SELF) + RUN(4,'x') => "abcdxxxx"
	var winBody bytes.Buffer
	winBody.Write(appendVCDInt(nil, 8)) // target window len
	winBody.WriteByte(0)                // delta indicator
	winBody.Write(appendVCDInt(nil, 1)) // dataLen = 1 ('x')
	winBody.Write(appendVCDInt(nil, 3)) // instLen = 3 (COPY 4 mode 0; RUN 4 => op 0, len 4)
	winBody.Write(appendVCDInt(nil, 1)) // addrLen = 1 (addr 0)
	winBody.WriteByte('x')              // data
	winBody.WriteByte(vcdSingleCOPY[0][4])
	winBody.WriteByte(0) // RUN
	winBody.Write(appendVCDInt(nil, 4))
	winBody.Write(appendVCDInt(nil, 0)) // VCD_SELF addr = 0

	var raw bytes.Buffer
	raw.Write(vcdiffMagic[:])
	raw.WriteByte(0)
	raw.WriteByte(vcdSource)
	raw.Write(appendVCDInt(nil, 6))
	raw.Write(appendVCDInt(nil, 0))
	raw.Write(appendVCDInt(nil, uint64(winBody.Len())))
	raw.Write(winBody.Bytes())

	var out bytes.Buffer
	if err := decodeRawVCDIFF(&out, 8, bytes.NewReader(old), int64(len(old)), &raw); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "abcdxxxx" {
		t.Fatalf("decodeRawVCDIFF = %q, want %q", got, "abcdxxxx")
	}
}

func FuzzDecodeVCDIFF(f *testing.F) {
	f.Add([]byte("old file"), EncodeVCDIFF([]byte("old file"), []byte("new file, longer")))
	f.Add([]byte("old file"), EncodeVCDIFF([]byte("old file, longer"), []byte("new file, longer")))
	f.Fuzz(func(t *testing.T, old, patch []byte) {
		var out bytes.Buffer
		const size = 16
		err := DecodeVCDIFF(&out, size, bytes.NewReader(old), int64(len(old)), bytes.NewReader(patch))
		if out.Len() > size || (err == nil && out.Len() != size) {
			t.Errorf("patch wrote %d bytes, want %d (err=%v)", out.Len(), size, err)
		}
	})
}
