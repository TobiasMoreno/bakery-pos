package inventory

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const maxQuantityMilliunits int64 = 999_999_999_999_999_999

var ErrInvalidQuantity = errors.New("invalid inventory quantity")

// Quantity stores an exact inventory amount in thousandths of its base unit.
// JSON and PostgreSQL boundaries use decimal strings so no float participates
// in inventory decisions.
type Quantity struct {
	milliunits int64
}

func ParseQuantity(value string) (Quantity, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return Quantity{}, ErrInvalidQuantity
	}

	sign := int64(1)
	if value[0] == '-' {
		sign = -1
		value = value[1:]
	} else if value[0] == '+' {
		value = value[1:]
	}
	if value == "" {
		return Quantity{}, ErrInvalidQuantity
	}

	parts := strings.Split(value, ".")
	if len(parts) > 2 || parts[0] == "" || (len(parts) == 2 && parts[1] == "") {
		return Quantity{}, ErrInvalidQuantity
	}
	if len(parts[0]) > 15 || !decimalDigits(parts[0]) {
		return Quantity{}, ErrInvalidQuantity
	}

	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
		if len(fraction) > 3 || !decimalDigits(fraction) {
			return Quantity{}, ErrInvalidQuantity
		}
	}
	fraction += strings.Repeat("0", 3-len(fraction))

	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return Quantity{}, ErrInvalidQuantity
	}
	fractional, err := strconv.ParseInt(fraction, 10, 64)
	if err != nil {
		return Quantity{}, ErrInvalidQuantity
	}
	milliunits := whole*1000 + fractional
	if milliunits > maxQuantityMilliunits {
		return Quantity{}, ErrInvalidQuantity
	}
	return Quantity{milliunits: sign * milliunits}, nil
}

func QuantityFromMilliunits(milliunits int64) (Quantity, error) {
	if milliunits > maxQuantityMilliunits || milliunits < -maxQuantityMilliunits {
		return Quantity{}, ErrInvalidQuantity
	}
	return Quantity{milliunits: milliunits}, nil
}

func (q Quantity) Milliunits() int64 {
	return q.milliunits
}

func (q Quantity) IsZero() bool {
	return q.milliunits == 0
}

func (q Quantity) IsInteger() bool {
	return q.milliunits%1000 == 0
}

func (q Quantity) String() string {
	value := q.milliunits
	sign := ""
	if value < 0 {
		sign = "-"
		value = -value
	}
	whole := value / 1000
	fraction := value % 1000
	if fraction == 0 {
		return fmt.Sprintf("%s%d", sign, whole)
	}
	return strings.TrimRight(fmt.Sprintf("%s%d.%03d", sign, whole, fraction), "0")
}

func (q Quantity) MarshalJSON() ([]byte, error) {
	return json.Marshal(q.String())
}

func (q *Quantity) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, []byte("null")) {
		return ErrInvalidQuantity
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("%w: quantity must be a decimal string", ErrInvalidQuantity)
	}
	parsed, err := ParseQuantity(value)
	if err != nil {
		return err
	}
	*q = parsed
	return nil
}

func decimalDigits(value string) bool {
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return value != ""
}
