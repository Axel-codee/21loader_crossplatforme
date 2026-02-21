package xuuid

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
)

type UUID string

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)

func New() UUID {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	hexstr := hex.EncodeToString(b[:])
	id := hexstr[0:8] + "-" + hexstr[8:12] + "-" + hexstr[12:16] + "-" + hexstr[16:20] + "-" + hexstr[20:32]
	return UUID(id)
}

func Parse(v string) (UUID, error) {
	v = strings.TrimSpace(v)
	if !uuidRe.MatchString(v) {
		return "", errors.New("invalid UUID")
	}
	return UUID(strings.ToLower(v)), nil
}

func (u UUID) String() string {
	return string(u)
}
