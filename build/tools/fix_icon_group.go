//go:build ignore

package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

const (
	rtGroupIcon = 14
)

func main() {
	if len(os.Args) != 2 {
		fatalf("usage: go run ./build/tools/fix_icon_group.go <rsrc.syso>")
	}
	path := os.Args[1]
	data, err := os.ReadFile(path)
	if err != nil {
		fatalf("%v", err)
	}
	base, err := coffResourceOffset(data)
	if err != nil {
		fatalf("%v", err)
	}
	if err := forceGroupIconID(data, base); err != nil {
		fatalf("%v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		fatalf("%v", err)
	}
}

func coffResourceOffset(data []byte) (int, error) {
	if len(data) < 20 {
		return 0, fmt.Errorf("file too small")
	}
	sections := int(u16(data, 2))
	optionalHeaderSize := int(u16(data, 16))
	sectionTable := 20 + optionalHeaderSize
	for i := 0; i < sections; i++ {
		off := sectionTable + i*40
		if off+40 > len(data) {
			return 0, fmt.Errorf("invalid COFF section table")
		}
		name := string(bytesUntilZero(data[off : off+8]))
		if name == ".rsrc" {
			rawSize := int(u32(data, off+16))
			rawOffset := int(u32(data, off+20))
			if rawOffset <= 0 || rawOffset+rawSize > len(data) {
				return 0, fmt.Errorf("invalid .rsrc section bounds")
			}
			return rawOffset, nil
		}
	}
	return 0, fmt.Errorf(".rsrc section not found")
}

func forceGroupIconID(data []byte, base int) error {
	rootEntries, err := resourceEntries(data, base, 0)
	if err != nil {
		return err
	}
	for _, root := range rootEntries {
		if root.id != rtGroupIcon || !root.isDir {
			continue
		}
		nameEntries, err := resourceEntries(data, base, root.target)
		if err != nil {
			return err
		}
		if len(nameEntries) == 0 {
			return fmt.Errorf("RT_GROUP_ICON has no entries")
		}
		first := nameEntries[0]
		if first.isName {
			return fmt.Errorf("RT_GROUP_ICON entry is named, expected numeric id")
		}
		binary.LittleEndian.PutUint32(data[first.entryOffset:first.entryOffset+4], 1)
		return nil
	}
	return fmt.Errorf("RT_GROUP_ICON not found")
}

type resourceEntry struct {
	id          uint32
	isName      bool
	isDir       bool
	target      uint32
	entryOffset int
}

func resourceEntries(data []byte, base int, rel uint32) ([]resourceEntry, error) {
	dir := base + int(rel)
	if dir+16 > len(data) {
		return nil, fmt.Errorf("invalid resource directory")
	}
	named := int(u16(data, dir+12))
	ids := int(u16(data, dir+14))
	count := named + ids
	entries := make([]resourceEntry, 0, count)
	for i := 0; i < count; i++ {
		entryOffset := dir + 16 + i*8
		if entryOffset+8 > len(data) {
			return nil, fmt.Errorf("invalid resource entry")
		}
		nameRaw := u32(data, entryOffset)
		dataRaw := u32(data, entryOffset+4)
		entries = append(entries, resourceEntry{
			id:          nameRaw & 0x7fffffff,
			isName:      nameRaw&0x80000000 != 0,
			isDir:       dataRaw&0x80000000 != 0,
			target:      dataRaw & 0x7fffffff,
			entryOffset: entryOffset,
		})
	}
	return entries, nil
}

func bytesUntilZero(b []byte) []byte {
	for i, c := range b {
		if c == 0 {
			return b[:i]
		}
	}
	return b
}

func u16(data []byte, off int) uint16 {
	return binary.LittleEndian.Uint16(data[off : off+2])
}

func u32(data []byte, off int) uint32 {
	return binary.LittleEndian.Uint32(data[off : off+4])
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
