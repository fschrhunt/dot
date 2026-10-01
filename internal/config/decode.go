package config

import (
	"math/big"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// integer preserves Python's arbitrary-precision TOML integers beyond Go's int64 range.
type integer string

// decode restores Python's numeric range while leaving all syntax parsing to the TOML parser.
func decode(src string, raw *map[string]any) (toml.MetaData, error) {
	type edit struct{ start, old, new int }
	text := src
	var edits []edit
	replacements := map[string]any{}
	for {
		md, err := toml.Decode(text, raw)
		if err == nil {
			if err := duplicateTable(src, md); err != nil {
				return md, err
			}
			var restore func(any) any
			restore = func(v any) any {
				switch x := v.(type) {
				case string:
					if original, ok := replacements[x]; ok {
						return original
					}
				case map[string]any:
					for k, v := range x {
						x[k] = restore(v)
					}
				case []any:
					for i, v := range x {
						x[i] = restore(v)
					}
				}
				return v
			}
			restore(*raw)
			return md, nil
		}
		p, ok := err.(toml.ParseError)
		if !ok {
			return md, err
		}
		start, end := p.Position.Start, p.Position.Start+p.Position.Len
		if !strings.Contains(p.Message, "is out of range for") || start < 0 || end > len(text) {
			for i := len(edits) - 1; i >= 0; i-- {
				e := edits[i]
				if p.Position.Start >= e.start+e.new {
					p.Position.Start += e.old - e.new
				} else if p.Position.Start > e.start {
					p.Position.Start = e.start + min(p.Position.Start-e.start, e.old)
				}
			}
			return md, p
		}
		token := strings.ReplaceAll(text[start:end], "_", "")
		var value any
		if strings.HasSuffix(p.Message, "for int64") {
			base := 10
			if strings.HasPrefix(token, "0x") || strings.HasPrefix(token, "0o") || strings.HasPrefix(token, "0b") {
				base = 0
			}
			n, ok := new(big.Int).SetString(token, base)
			if !ok {
				return md, err
			}
			value = integer(n.String())
		} else {
			n, _ := strconv.ParseFloat(token, 64)
			value = n
		}
		marker := "dot_numeric_" + strconv.Itoa(len(edits))
		for strings.Contains(src, marker) {
			marker += "_"
		}
		replacements[marker] = value
		replacement := strconv.Quote(marker)
		text = text[:start] + replacement + text[end:]
		edits = append(edits, edit{start, end - start, len(replacement)})
	}
}

// duplicateTable rejects empty arrays redeclared as tables, which tomllib forbids.
func duplicateTable(src string, md toml.MetaData) error {
	seen := map[string]bool{}
	for _, key := range md.Keys() {
		name := key.String()
		array := false
		for n := 1; n <= len(key); n++ {
			if md.Type(key[:n]...) == "ArrayHash" {
				array = true
			}
		}
		if seen[name] && md.Type(key...) == "Hash" && !array {
			pos, offset := 0, 0
			for _, line := range strings.SplitAfter(src, "\n") {
				header := strings.TrimSpace(line)
				if strings.HasPrefix(header, "[") && !strings.HasPrefix(header, "[[") {
					var table map[string]any
					meta, err := toml.Decode(header, &table)
					if err == nil && len(meta.Keys()) == 1 && meta.Keys()[0].String() == name {
						pos = offset + strings.Index(line, "[") + 1
					}
				}
				offset += len(line)
			}
			return toml.ParseError{Message: "Key '" + name + "' has already been defined.", Position: toml.Position{Start: pos}}
		}
		seen[name] = true
	}
	return nil
}
