package datautil

import (
	"encoding/hex"
	"fmt"
	"math/rand"
)

type RandomFiller struct {
	rng *rand.Rand
}

func NewRandomFiller(seed int64) *RandomFiller {
	randSource := rand.NewSource(seed)
	rng := rand.New(randSource)

	return &RandomFiller{rng}
}

func (f *RandomFiller) Fill(buf []byte) {
	for i := range buf {
		buf[i] = byte(f.rng.Int31())
	}
}

type MismatchDetails struct {
	offsets []int
}

func (m *MismatchDetails) HasErrors() bool {
	return len(m.offsets) > 0
}

func (m *MismatchDetails) Offsets() []int {
	return m.offsets
}

// Return true if in and expected are the same
func MatchWithErrors(in []byte, expected []byte, dumpOnError bool) *MismatchDetails {
	isValid := true
	if len(in) != len(expected) {
		fmt.Printf("ERROR: received length=%d; expected=%d\n", len(in), len(expected))
		isValid = false
	}

	result := MismatchDetails{}
	for i, b := range in {
		if b != expected[i] {
			fmt.Printf("ERROR: byte at position %d (0x%x) should be 0x%02x; is actually 0x%02x\n",
				i, i, expected[i], b)
			result.offsets = append(result.offsets, i)
			isValid = false
		}
	}
	if !isValid && dumpOnError {
		fmt.Println("Incorrect data:")
		// fmt.Println(hex.EncodeToString(buf))
		fmt.Println(hex.Dump(in))
		fmt.Println("Expected data:")
		fmt.Println(hex.Dump(expected))
	}
	return &result
}
