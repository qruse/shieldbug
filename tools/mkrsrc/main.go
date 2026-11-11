// Command mkrsrc turns a .ico file into COFF resource objects (.syso) that
// the Go linker embeds in Windows builds, so shieldbug.exe gets its icon.
// Standard library only, so building Shieldbug never pulls in outside code.
//
//	go run ./tools/mkrsrc assets/icon.ico
//
// writes rsrc_windows_amd64.syso and rsrc_windows_arm64.syso in the current folder.
package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
)

type icoImage struct {
	w, h     byte
	colors   byte
	planes   uint16
	bitCount uint16
	data     []byte
}

func readICO(path string) ([]icoImage, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(b) < 6 || binary.LittleEndian.Uint16(b[2:]) != 1 {
		return nil, errors.New("not an .ico file")
	}
	n := int(binary.LittleEndian.Uint16(b[4:]))
	var imgs []icoImage
	for i := 0; i < n; i++ {
		e := b[6+16*i:]
		size := binary.LittleEndian.Uint32(e[8:])
		off := binary.LittleEndian.Uint32(e[12:])
		if int(off+size) > len(b) {
			return nil, fmt.Errorf("image %d is truncated", i)
		}
		imgs = append(imgs, icoImage{w: e[0], h: e[1], colors: e[2],
			planes: binary.LittleEndian.Uint16(e[4:]), bitCount: binary.LittleEndian.Uint16(e[6:]),
			data: b[off : off+size]})
	}
	return imgs, nil
}

const (
	rtIcon      = 3
	rtGroupIcon = 14
	langNeutral = 0x0409 // en-US, what resource compilers emit by default
)

// buildRsrc lays out a resource tree with one RT_GROUP_ICON (ID 1) and one
// RT_ICON per image (IDs 1..n). It returns the section bytes and the offsets
// of the data-entry RVA fields that need ADDR32NB relocations.
func buildRsrc(imgs []icoImage) ([]byte, []uint32) {
	// Group icon directory (GRPICONDIR + GRPICONDIRENTRY[]).
	var grp bytes.Buffer
	binary.Write(&grp, binary.LittleEndian, []uint16{0, 1, uint16(len(imgs))})
	for i, im := range imgs {
		grp.Write([]byte{im.w, im.h, im.colors, 0})
		binary.Write(&grp, binary.LittleEndian, im.planes)
		binary.Write(&grp, binary.LittleEndian, im.bitCount)
		binary.Write(&grp, binary.LittleEndian, uint32(len(im.data)))
		binary.Write(&grp, binary.LittleEndian, uint16(i+1))
	}

	type leaf struct{ data []byte }
	icons := make([]leaf, len(imgs))
	for i, im := range imgs {
		icons[i] = leaf{im.data}
	}

	// Sizes of each table so offsets can be computed up front.
	dir := func(entries int) int { return 16 + 8*entries }
	n := len(imgs)
	root := 0
	typeIcon := root + dir(2)
	typeGroup := typeIcon + dir(n)
	langIcon := typeGroup + dir(1) // n language directories, one entry each
	langGroup := langIcon + n*dir(1)
	dataEntries := langGroup + dir(1) // n+1 data entries, 16 bytes each
	blobs := dataEntries + 16*(n+1)

	out := make([]byte, blobs)
	put32 := func(off int, v uint32) { binary.LittleEndian.PutUint32(out[off:], v) }
	put16 := func(off int, v uint16) { binary.LittleEndian.PutUint16(out[off:], v) }
	writeDir := func(off, ids int) { put16(off+14, uint16(ids)) }
	entry := func(dirOff, idx int, id uint32, target int, sub bool) {
		e := dirOff + 16 + 8*idx
		put32(e, id)
		t := uint32(target)
		if sub {
			t |= 0x80000000
		}
		put32(e+4, t)
	}

	writeDir(root, 2)
	entry(root, 0, rtIcon, typeIcon, true)
	entry(root, 1, rtGroupIcon, typeGroup, true)

	writeDir(typeIcon, n)
	for i := 0; i < n; i++ {
		entry(typeIcon, i, uint32(i+1), langIcon+i*dir(1), true)
	}
	writeDir(typeGroup, 1)
	entry(typeGroup, 0, 1, langGroup, true)

	for i := 0; i < n; i++ {
		d := langIcon + i*dir(1)
		writeDir(d, 1)
		entry(d, 0, langNeutral, dataEntries+16*i, false)
	}
	writeDir(langGroup, 1)
	entry(langGroup, 0, langNeutral, dataEntries+16*n, false)

	var relocs []uint32
	blobsData := append([][]byte{}, func() [][]byte {
		var s [][]byte
		for _, l := range icons {
			s = append(s, l.data)
		}
		return append(s, grp.Bytes())
	}()...)
	for i, data := range blobsData {
		for len(out)%8 != 0 {
			out = append(out, 0)
		}
		start := len(out)
		out = append(out, data...)
		de := dataEntries + 16*i
		put32(de, uint32(start)) // section-relative; the relocation adds the section RVA
		put32(de+4, uint32(len(data)))
		relocs = append(relocs, uint32(de))
	}
	for len(out)%8 != 0 {
		out = append(out, 0)
	}
	return out, relocs
}

func writeCOFF(path string, machine, relocType uint16, sect []byte, relocs []uint32) error {
	var b bytes.Buffer
	le := binary.LittleEndian
	const fileHdr, sectHdr = 20, 40
	dataOff := uint32(fileHdr + sectHdr)
	relocOff := dataOff + uint32(len(sect))
	symOff := relocOff + uint32(10*len(relocs))

	// IMAGE_FILE_HEADER
	binary.Write(&b, le, machine)
	binary.Write(&b, le, uint16(1)) // sections
	binary.Write(&b, le, uint32(0)) // timestamp
	binary.Write(&b, le, symOff)
	binary.Write(&b, le, uint32(1)) // symbols
	binary.Write(&b, le, uint16(0)) // optional header size
	binary.Write(&b, le, uint16(0)) // characteristics

	// IMAGE_SECTION_HEADER
	b.Write([]byte(".rsrc\x00\x00\x00"))
	binary.Write(&b, le, uint32(0)) // virtual size
	binary.Write(&b, le, uint32(0)) // virtual address
	binary.Write(&b, le, uint32(len(sect)))
	binary.Write(&b, le, dataOff)
	binary.Write(&b, le, relocOff)
	binary.Write(&b, le, uint32(0)) // line numbers
	binary.Write(&b, le, uint16(len(relocs)))
	binary.Write(&b, le, uint16(0))
	binary.Write(&b, le, uint32(0x40000040)) // initialized data, readable

	b.Write(sect)
	for _, r := range relocs {
		binary.Write(&b, le, r)
		binary.Write(&b, le, uint32(0)) // symbol 0 = .rsrc
		binary.Write(&b, le, relocType)
	}

	// Symbol table: one static symbol for the section, then an empty string table.
	b.Write([]byte(".rsrc\x00\x00\x00"))
	binary.Write(&b, le, uint32(0))
	binary.Write(&b, le, int16(1))
	binary.Write(&b, le, uint16(0))
	b.Write([]byte{3, 0}) // IMAGE_SYM_CLASS_STATIC, no aux
	binary.Write(&b, le, uint32(4))

	return os.WriteFile(path, b.Bytes(), 0o644)
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./tools/mkrsrc <icon.ico>")
		os.Exit(2)
	}
	imgs, err := readICO(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sect, relocs := buildRsrc(imgs)
	targets := []struct {
		file           string
		machine, reloc uint16
	}{
		{"rsrc_windows_amd64.syso", 0x8664, 3}, // IMAGE_REL_AMD64_ADDR32NB
		{"rsrc_windows_arm64.syso", 0xAA64, 2}, // IMAGE_REL_ARM64_ADDR32NB
	}
	for _, t := range targets {
		if err := writeCOFF(t.file, t.machine, t.reloc, sect, relocs); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s (%d icons, %d bytes of resources)\n", t.file, len(imgs), len(sect))
	}
}
