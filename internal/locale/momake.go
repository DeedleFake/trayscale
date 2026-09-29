package locale

import (
	"bytes"
	"encoding/binary"
	"sort"
	"strings"

	"github.com/leonelquinteros/gotext"
)

const (
	moMagicLE    = 0x950412de
	moHeaderSize = 28
	eotSep       = "\x04"
	nulSep       = "\x00"
)

// moEntry is one msgid/msgstr pair ready for the MO tables.
type moEntry struct {
	id  string // encoded original (may contain interior NUL / EOT)
	str string // encoded translation (plurals joined with NUL)
}

// compilePOtoMO parses GNU gettext PO bytes and returns a little-endian MO catalog.
func compilePOtoMO(poData []byte) ([]byte, error) {
	po := gotext.NewPo()
	po.Parse(poData)
	return writeMO(po.GetDomain())
}

func writeMO(domain *gotext.Domain) ([]byte, error) {
	entries := collectEntries(domain)
	sort.Slice(entries, func(i, j int) bool { return entries[i].id < entries[j].id })

	n := uint32(len(entries))
	hashSize := moHashSize(n)

	// Layout: header | orig table | trans table | hash table | strings
	origTabOff := uint32(moHeaderSize)
	transTabOff := origTabOff + n*8
	hashOff := transTabOff + n*8
	strOff := hashOff + hashSize*4

	type desc struct{ length, offset uint32 }
	orig := make([]desc, n)
	trans := make([]desc, n)
	var strs bytes.Buffer

	writeStr := func(s string) (length, offset uint32) {
		offset = strOff + uint32(strs.Len())
		length = uint32(len(s))
		strs.WriteString(s)
		strs.WriteByte(0)
		return length, offset
	}

	for i, e := range entries {
		orig[i].length, orig[i].offset = writeStr(e.id)
		trans[i].length, trans[i].offset = writeStr(e.str)
	}

	hashTab := buildMOHash(entries, hashSize)

	var out bytes.Buffer
	out.Grow(int(strOff) + strs.Len())

	_ = binary.Write(&out, binary.LittleEndian, uint32(moMagicLE))
	_ = binary.Write(&out, binary.LittleEndian, uint32(0)) // revision
	_ = binary.Write(&out, binary.LittleEndian, n)
	_ = binary.Write(&out, binary.LittleEndian, origTabOff)
	_ = binary.Write(&out, binary.LittleEndian, transTabOff)
	_ = binary.Write(&out, binary.LittleEndian, hashSize)
	_ = binary.Write(&out, binary.LittleEndian, hashOff)

	for _, d := range orig {
		_ = binary.Write(&out, binary.LittleEndian, d.length)
		_ = binary.Write(&out, binary.LittleEndian, d.offset)
	}
	for _, d := range trans {
		_ = binary.Write(&out, binary.LittleEndian, d.length)
		_ = binary.Write(&out, binary.LittleEndian, d.offset)
	}
	for _, h := range hashTab {
		_ = binary.Write(&out, binary.LittleEndian, h)
	}
	_, _ = out.Write(strs.Bytes())
	return out.Bytes(), nil
}

func collectEntries(domain *gotext.Domain) []moEntry {
	var out []moEntry
	seen := make(map[string]struct{})

	add := func(ctx string, tr *gotext.Translation) {
		id := encodeMsgid(ctx, tr)
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, moEntry{id: id, str: encodeMsgstr(tr)})
	}

	for _, tr := range domain.GetTranslations() {
		add("", tr)
	}
	for ctx, m := range domain.GetCtxTranslations() {
		for _, tr := range m {
			add(ctx, tr)
		}
	}
	return out
}

func encodeMsgid(ctx string, tr *gotext.Translation) string {
	id := tr.ID
	if ctx != "" {
		id = ctx + eotSep + id
	}
	if tr.PluralID != "" {
		id = id + nulSep + tr.PluralID
	}
	return id
}

func encodeMsgstr(tr *gotext.Translation) string {
	if tr.PluralID == "" {
		if s, ok := tr.Trs[0]; ok {
			return s
		}
		return ""
	}
	max := -1
	for i := range tr.Trs {
		if i > max {
			max = i
		}
	}
	if max < 0 {
		return ""
	}
	parts := make([]string, max+1)
	for i := 0; i <= max; i++ {
		parts[i] = tr.Trs[i]
	}
	return strings.Join(parts, nulSep)
}

// moHashSize returns a prime hash table size as msgfmt does (~4/3 * n).
func moHashSize(n uint32) uint32 {
	if n == 0 {
		return 0
	}
	size := nextPrime((n * 4) / 3)
	if size <= 2 {
		return 3
	}
	return size
}

func nextPrime(n uint32) uint32 {
	if n < 2 {
		return 2
	}
	for {
		if isPrime(n) {
			return n
		}
		n++
	}
}

func isPrime(n uint32) bool {
	if n < 2 {
		return false
	}
	if n%2 == 0 {
		return n == 2
	}
	for i := uint32(3); i*i <= n; i += 2 {
		if n%i == 0 {
			return false
		}
	}
	return true
}

// hashPJW is gettext's hash_string (PJW), used for the MO hash table.
// It stops at the first NUL, matching libintl over plural msgids.
func hashPJW(s string) uint32 {
	var hval uint32
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == 0 {
			break
		}
		hval = (hval << 4) + uint32(c)
		if g := hval & 0xf0000000; g != 0 {
			hval ^= g >> 24
			hval ^= g
		}
	}
	return hval
}

func buildMOHash(entries []moEntry, hashSize uint32) []uint32 {
	if hashSize == 0 {
		return nil
	}
	tab := make([]uint32, hashSize)
	for i, e := range entries {
		h := hashPJW(e.id)
		idx := h % hashSize
		incr := 1 + (h % (hashSize - 2))
		for tab[idx] != 0 {
			if idx >= hashSize-incr {
				idx -= hashSize - incr
			} else {
				idx += incr
			}
		}
		tab[idx] = uint32(i + 1) // 1-based index into sorted string table
	}
	return tab
}
