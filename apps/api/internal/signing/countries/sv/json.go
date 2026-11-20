package sv

import (
	"errors"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"
)

// What the TypeScript signer signed was JSON.stringify(JSON.parse(body)).
// This file reproduces that round trip: a JSON parser as strict as
// JSON.parse that keeps what JavaScript keeps, and a writer that writes what
// JSON.stringify writes.
//
//   - Object keys keep their first position, the last value wins, and the
//     keys that are array indexes ("0" to "4294967294", canonical) come first,
//     in numeric order, as in any JavaScript object.
//   - Numbers are IEEE 754 doubles written as Number.prototype.toString
//     writes them (1.0 → 1, 1e21 → 1e+21, 1e-7 → 1e-7, -0 → 0); one too
//     large for a double is Infinity, which JSON.stringify writes null.
//   - Strings escape only '"', '\\', the controls below U+0020 (\b \f \n \r
//     \t or \u00XX) and lone surrogates, which are kept (as WTF-8) and
//     written \uDXXX.

var errJSON = errors.New("not valid JSON")

type jsonValue any // nil, bool, float64, string, []jsonValue, *jsonObject

type jsonObject struct {
	keys   []string
	values map[string]jsonValue
}

func (o *jsonObject) get(key string) (jsonValue, bool) {
	v, ok := o.values[key]
	return v, ok
}

type jsonParser struct {
	s   string
	pos int
}

func parseJSON(s string) (jsonValue, error) {
	p := &jsonParser{s: s}
	p.space()
	v, err := p.value()
	if err != nil {
		return nil, err
	}
	p.space()
	if p.pos != len(p.s) {
		return nil, errJSON
	}
	return v, nil
}

func (p *jsonParser) space() {
	for p.pos < len(p.s) {
		switch p.s[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.pos++
		default:
			return
		}
	}
}

func (p *jsonParser) value() (jsonValue, error) {
	if p.pos >= len(p.s) {
		return nil, errJSON
	}
	switch c := p.s[p.pos]; {
	case c == '{':
		return p.object()
	case c == '[':
		return p.array()
	case c == '"':
		return p.string()
	case c == '-' || c >= '0' && c <= '9':
		return p.number()
	case strings.HasPrefix(p.s[p.pos:], "true"):
		p.pos += 4
		return true, nil
	case strings.HasPrefix(p.s[p.pos:], "false"):
		p.pos += 5
		return false, nil
	case strings.HasPrefix(p.s[p.pos:], "null"):
		p.pos += 4
		return nil, nil
	}
	return nil, errJSON
}

func (p *jsonParser) object() (jsonValue, error) {
	p.pos++
	o := &jsonObject{values: map[string]jsonValue{}}
	p.space()
	if p.pos < len(p.s) && p.s[p.pos] == '}' {
		p.pos++
		return o, nil
	}
	for {
		p.space()
		if p.pos >= len(p.s) || p.s[p.pos] != '"' {
			return nil, errJSON
		}
		k, err := p.string()
		if err != nil {
			return nil, err
		}
		p.space()
		if p.pos >= len(p.s) || p.s[p.pos] != ':' {
			return nil, errJSON
		}
		p.pos++
		p.space()
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		key := k.(string)
		if _, seen := o.values[key]; !seen {
			o.keys = append(o.keys, key)
		}
		o.values[key] = v
		p.space()
		if p.pos >= len(p.s) {
			return nil, errJSON
		}
		switch p.s[p.pos] {
		case ',':
			p.pos++
		case '}':
			p.pos++
			return o, nil
		default:
			return nil, errJSON
		}
	}
}

func (p *jsonParser) array() (jsonValue, error) {
	p.pos++
	a := []jsonValue{}
	p.space()
	if p.pos < len(p.s) && p.s[p.pos] == ']' {
		p.pos++
		return a, nil
	}
	for {
		p.space()
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		a = append(a, v)
		p.space()
		if p.pos >= len(p.s) {
			return nil, errJSON
		}
		switch p.s[p.pos] {
		case ',':
			p.pos++
		case ']':
			p.pos++
			return a, nil
		default:
			return nil, errJSON
		}
	}
}

func (p *jsonParser) number() (jsonValue, error) {
	start := p.pos
	if p.s[p.pos] == '-' {
		p.pos++
	}
	digits := func() int {
		n := 0
		for p.pos < len(p.s) && p.s[p.pos] >= '0' && p.s[p.pos] <= '9' {
			p.pos++
			n++
		}
		return n
	}
	if p.pos < len(p.s) && p.s[p.pos] == '0' {
		p.pos++
	} else if digits() == 0 {
		return nil, errJSON
	}
	if p.pos < len(p.s) && p.s[p.pos] == '.' {
		p.pos++
		if digits() == 0 {
			return nil, errJSON
		}
	}
	if p.pos < len(p.s) && (p.s[p.pos] == 'e' || p.s[p.pos] == 'E') {
		p.pos++
		if p.pos < len(p.s) && (p.s[p.pos] == '+' || p.s[p.pos] == '-') {
			p.pos++
		}
		if digits() == 0 {
			return nil, errJSON
		}
	}
	f, err := strconv.ParseFloat(p.s[start:p.pos], 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		return nil, errJSON
	}
	return f, nil
}

func (p *jsonParser) string() (jsonValue, error) {
	p.pos++
	var b strings.Builder
	for {
		if p.pos >= len(p.s) {
			return nil, errJSON
		}
		c := p.s[p.pos]
		switch {
		case c == '"':
			p.pos++
			return b.String(), nil
		case c < 0x20:
			return nil, errJSON
		case c != '\\':
			b.WriteByte(c)
			p.pos++
			continue
		}
		p.pos++
		if p.pos >= len(p.s) {
			return nil, errJSON
		}
		e := p.s[p.pos]
		p.pos++
		switch e {
		case '"', '\\', '/':
			b.WriteByte(e)
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'u':
			u, ok := p.hex4()
			if !ok {
				return nil, errJSON
			}
			if utf16.IsSurrogate(rune(u)) && u < 0xDC00 && strings.HasPrefix(p.s[p.pos:], `\u`) {
				save := p.pos
				p.pos += 2
				if lo, ok := p.hex4(); ok && lo >= 0xDC00 && lo <= 0xDFFF {
					b.WriteRune(utf16.DecodeRune(rune(u), rune(lo)))
					continue
				}
				p.pos = save
			}
			writeWTF8(&b, rune(u))
		default:
			return nil, errJSON
		}
	}
}

func (p *jsonParser) hex4() (uint16, bool) {
	if p.pos+4 > len(p.s) {
		return 0, false
	}
	n, err := strconv.ParseUint(p.s[p.pos:p.pos+4], 16, 16)
	if err != nil {
		return 0, false
	}
	p.pos += 4
	return uint16(n), true
}

// writeWTF8 writes r, a lone surrogate included, in UTF-8's three-byte form.
func writeWTF8(b *strings.Builder, r rune) {
	if !utf16.IsSurrogate(r) {
		b.WriteRune(r)
		return
	}
	b.WriteByte(byte(0xE0 | r>>12))
	b.WriteByte(byte(0x80 | (r>>6)&0x3F))
	b.WriteByte(byte(0x80 | r&0x3F))
}

// stringify writes v as JSON.stringify does.
func stringify(b *strings.Builder, v jsonValue) {
	switch t := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		b.WriteString(strconv.FormatBool(t))
	case float64:
		b.WriteString(jsNumber(t))
	case string:
		writeString(b, t)
	case []jsonValue:
		b.WriteByte('[')
		for i, e := range t {
			if i > 0 {
				b.WriteByte(',')
			}
			stringify(b, e)
		}
		b.WriteByte(']')
	case *jsonObject:
		b.WriteByte('{')
		for i, k := range jsKeyOrder(t.keys) {
			if i > 0 {
				b.WriteByte(',')
			}
			writeString(b, k)
			b.WriteByte(':')
			stringify(b, t.values[k])
		}
		b.WriteByte('}')
	}
}

// jsKeyOrder is the order of a JavaScript object's own string keys: array
// indexes ascending, then the rest as they were added.
func jsKeyOrder(keys []string) []string {
	var indexes, rest []string
	for _, k := range keys {
		if isArrayIndex(k) {
			indexes = append(indexes, k)
		} else {
			rest = append(rest, k)
		}
	}
	slices.SortFunc(indexes, func(a, b string) int {
		x, _ := strconv.ParseUint(a, 10, 32)
		y, _ := strconv.ParseUint(b, 10, 32)
		return int(int64(x) - int64(y))
	})
	return append(indexes, rest...)
}

func isArrayIndex(k string) bool {
	if k == "" || len(k) > 10 || (len(k) > 1 && k[0] == '0') {
		return false
	}
	for i := 0; i < len(k); i++ {
		if k[i] < '0' || k[i] > '9' {
			return false
		}
	}
	n, err := strconv.ParseUint(k, 10, 64)
	return err == nil && n < math.MaxUint32
}

func writeString(b *strings.Builder, s string) {
	b.WriteByte('"')
	for i := 0; i < len(s); {
		c := s[i]
		// A lone surrogate, as writeWTF8 wrote it.
		if c == 0xED && i+2 < len(s) && s[i+1] >= 0xA0 && s[i+1] <= 0xBF {
			r := rune(c&0x0F)<<12 | rune(s[i+1]&0x3F)<<6 | rune(s[i+2]&0x3F)
			b.WriteString(`\u` + strconv.FormatInt(int64(r), 16))
			i += 3
			continue
		}
		switch c {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if c < 0x20 {
				b.WriteString(`\u00`)
				b.WriteString(strconv.FormatInt(int64(c>>4), 16) + strconv.FormatInt(int64(c&0xF), 16))
			} else {
				b.WriteByte(c)
			}
		}
		i++
	}
	b.WriteByte('"')
}

// jsNumber writes f as Number.prototype.toString does; JSON.stringify
// writes a non-finite number as null.
func jsNumber(f float64) string {
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return "null"
	}
	if f == 0 {
		return "0"
	}
	sign := ""
	if f < 0 {
		sign, f = "-", -f
	}
	// The shortest digits that round-trip, and n: f = 0.digits × 10^n.
	e := strconv.FormatFloat(f, 'e', -1, 64)
	mant, exp, _ := strings.Cut(e, "e")
	digits := strings.Replace(mant, ".", "", 1)
	x, _ := strconv.Atoi(exp)
	k, n := len(digits), x+1
	var s string
	switch {
	case k <= n && n <= 21:
		s = digits + strings.Repeat("0", n-k)
	case 0 < n && n <= 21:
		s = digits[:n] + "." + digits[n:]
	case -6 < n && n <= 0:
		s = "0." + strings.Repeat("0", -n) + digits
	default:
		s = digits[:1]
		if k > 1 {
			s += "." + digits[1:]
		}
		if n-1 >= 0 {
			s += "e+" + strconv.Itoa(n-1)
		} else {
			s += "e-" + strconv.Itoa(1-n)
		}
	}
	return sign + s
}
