package colfer

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// GenerateGo writes the code into file "Colfer.go".
func GenerateGo(basedir string, packages []*Package) error {
	t := template.New("go-code")
	template.Must(t.Parse(goCode))
	template.Must(t.New("marshal-field").Parse(goMarshalField))
	template.Must(t.New("marshal-field-len").Parse(goMarshalFieldLen))
	template.Must(t.New("unmarshal-field").Parse(goUnmarshalField))
	template.Must(t.New("unmarshal-varint").Parse(goUnmarshalVarint))

	for _, p := range packages {
		p.NameNative = p.Name[strings.LastIndexByte(p.Name, '/')+1:]
	}

	for _, p := range packages {
		for _, s := range p.Structs {
			for _, f := range s.Fields {
				switch f.Type {
				default:
					if f.TypeRef == nil {
						f.TypeNative = f.Type
					} else {
						f.TypeNative = f.TypeRef.NameTitle()
						if f.TypeRef.Pkg != p {
							f.TypeNative = f.TypeRef.Pkg.NameNative + "." + f.TypeNative
						}
					}
				case "timestamp":
					f.TypeNative = "time.Time"
				case "text":
					f.TypeNative = "string"
				case "binary":
					f.TypeNative = "[]byte"
				}
			}
		}

		var buf bytes.Buffer
		if err := t.Execute(&buf, p); err != nil {
			return err
		}

		path := filepath.Join(basedir, p.Name)
		if err := os.MkdirAll(path, 0777); err != nil {
			return err
		}

		path = filepath.Join(path, "Colfer.go")
		if err := ioutil.WriteFile(path, buf.Bytes(), 0666); err != nil {
			return err
		}

		if _, err := Format(path); err != nil {
			return err
		}
	}
	return nil
}

const goCode = `{{.DocText "//"}}package {{.NameNative}}

// This file was generated by colf(1); DO NOT EDIT
// The compiler used schema file {{.SchemaFileList}}.

import (
	"encoding/binary"
	"fmt"
	"io"
{{- if .HasFloat}}
	"math"
{{- end}}
{{- if .HasTimestamp}}
	"time"
{{- end}}
{{- range .Refs}}
	"{{.Name}}"
{{- end}}
)

var intconv = binary.BigEndian

// Colfer configuration attributes
var (
	// ColferSizeMax is the upper limit for serial byte sizes.
	ColferSizeMax = {{.SizeMax}}
{{- if .HasList}}
	// ColferListMax is the upper limit for the number of elements in a list.
	ColferListMax = {{.ListMax}}
{{- end}}
)

// ColferMax signals an upper limit breach.
type ColferMax string

// Error honors the error interface.
func (m ColferMax) Error() string { return string(m) }

// ColferError signals a data mismatch as as a byte index.
type ColferError int

// Error honors the error interface.
func (i ColferError) Error() string {
	return fmt.Sprintf("colfer: unknown header at byte %d", i)
}

// ColferTail signals data continuation as a byte index.
type ColferTail int

// Error honors the error interface.
func (i ColferTail) Error() string {
	return fmt.Sprintf("colfer: data continuation at byte %d", i)
}
{{range .Structs}}
{{.DocText "//"}}type {{.NameTitle}} struct {
{{range .Fields}}{{.DocText "\t//"}}	{{.NameTitle}}	{{if .TypeList}}[]{{end}}{{if .TypeRef}}*{{end}}{{.TypeNative}}
{{end}}}

// MarshalTo encodes o as Colfer into buf and returns the number of bytes written.
// If the buffer is too small, MarshalTo will panic.
{{- range .Fields}}{{if and .TypeList .TypeRef}}
// All nil entries in o.{{.NameTitle}} will be replaced with a new value.
{{- end}}{{end}}
func (o *{{.NameTitle}}) MarshalTo(buf []byte) int {
	var i int
{{range .Fields}}{{template "marshal-field" .}}{{end}}
	buf[i] = 0x7f
	i++
	return i
}

// MarshalLen returns the Colfer serial byte size.
// The error return option is {{.Pkg.NameNative}}.ColferMax.
func (o *{{.NameTitle}}) MarshalLen() (int, error) {
	l := 1
{{range .Fields}}{{template "marshal-field-len" .}}{{end}}
	if l > ColferSizeMax {
		return l, ColferMax(fmt.Sprintf("colfer: struct {{.String}} exceeds %d bytes", ColferSizeMax))
	}
	return l, nil
}

// MarshalBinary encodes o as Colfer conform encoding.BinaryMarshaler.
{{- range .Fields}}{{if and .TypeList .TypeRef}}
// All nil entries in o.{{.NameTitle}} will be replaced with a new value.
{{- end}}{{end}}
// The error return option is {{.Pkg.NameNative}}.ColferMax.
func (o *{{.NameTitle}}) MarshalBinary() (data []byte, err error) {
	l, err := o.MarshalLen()
	if err != nil {
		return nil, err
	}
	data = make([]byte, l)
	o.MarshalTo(data)
	return data, nil
}

// Unmarshal decodes data as Colfer and returns the number of bytes read.
// The error return options are io.EOF, {{.Pkg.NameNative}}.ColferError and {{.Pkg.NameNative}}.ColferMax.
func (o *{{.NameTitle}}) Unmarshal(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, io.EOF
	}
	header := data[0]
	i := 1
{{range .Fields}}{{template "unmarshal-field" .}}{{end}}
	if header != 0x7f {
		return 0, ColferError(i - 1)
	}
	if i < ColferSizeMax {
		return i, nil
	}
eof:
	if i >= ColferSizeMax {
		return 0, ColferMax(fmt.Sprintf("colfer: {{.String}} size exceeds %d bytes", ColferSizeMax))
	}
	return 0, io.EOF
}

// UnmarshalBinary decodes data as Colfer conform encoding.BinaryUnmarshaler.
// The error return options are io.EOF, {{.Pkg.NameNative}}.ColferError, {{.Pkg.NameNative}}.ColferTail and {{.Pkg.NameNative}}.ColferMax.
func (o *{{.NameTitle}}) UnmarshalBinary(data []byte) error {
	i, err := o.Unmarshal(data)
	if i < len(data) && err == nil {
		return ColferTail(i)
	}
	return err
}
{{end}}`

const goMarshalField = `{{if eq .Type "bool"}}
	if o.{{.NameTitle}} {
		buf[i] = {{.Index}}
		i++
	}
{{else if eq .Type "uint8"}}
	if x := o.{{.NameTitle}}; x != 0 {
		buf[i] = {{.Index}}
		i++
		buf[i] = x
		i++
	}
{{else if eq .Type "uint16"}}
	if x := o.{{.NameTitle}}; x >= 1<<8 {
		buf[i] = {{.Index}}
		i++
		buf[i] = byte(x >> 8)
		i++
		buf[i] = byte(x)
		i++
	} else if x != 0 {
		buf[i] = {{.Index}} | 0x80
		i++
		buf[i] = byte(x)
		i++
	}
{{else if eq .Type "uint32"}}
	if x := o.{{.NameTitle}}; x >= 1<<21 {
		buf[i] = {{.Index}} | 0x80
		intconv.PutUint32(buf[i+1:], x)
		i += 5
	} else if x != 0 {
		buf[i] = {{.Index}}
		i++
		for x >= 0x80 {
			buf[i] = byte(x | 0x80)
			x >>= 7
			i++
		}
		buf[i] = byte(x)
		i++
	}
{{else if eq .Type "uint64"}}
	if x := o.{{.NameTitle}}; x >= 1<<49 {
		buf[i] = {{.Index}} | 0x80
		intconv.PutUint64(buf[i+1:], x)
		i += 9
	} else if x != 0 {
		buf[i] = {{.Index}}
		i++
		for x >= 0x80 {
			buf[i] = byte(x | 0x80)
			x >>= 7
			i++
		}
		buf[i] = byte(x)
		i++
	}
{{else if eq .Type "int32"}}
	if v := o.{{.NameTitle}}; v != 0 {
		x := uint32(v)
		if v >= 0 {
			buf[i] = {{.Index}}
		} else {
			x = ^x + 1
			buf[i] = {{.Index}} | 0x80
		}
		i++
		for x >= 0x80 {
			buf[i] = byte(x | 0x80)
			x >>= 7
			i++
		}
		buf[i] = byte(x)
		i++
	}
{{else if eq .Type "int64"}}
	if v := o.{{.NameTitle}}; v != 0 {
		x := uint64(v)
		if v >= 0 {
			buf[i] = {{.Index}}
		} else {
			x = ^x + 1
			buf[i] = {{.Index}} | 0x80
		}
		i++
		for n := 0; x >= 0x80 && n < 8; n++ {
			buf[i] = byte(x | 0x80)
			x >>= 7
			i++
		}
		buf[i] = byte(x)
		i++
	}
{{else if eq .Type "float32"}}
 {{- if .TypeList}}
	if l := len(o.{{.NameTitle}}); l != 0 {
		buf[i] = {{.Index}}
		i++
		x := uint(l)
		for x >= 0x80 {
			buf[i] = byte(x | 0x80)
			x >>= 7
			i++
		}
		buf[i] = byte(x)
		i++
		for _, v := range o.{{.NameTitle}} {
			intconv.PutUint32(buf[i:], math.Float32bits(v))
			i += 4
		}
	}
 {{- else}}
	if v := o.{{.NameTitle}}; v != 0 {
		buf[i] = {{.Index}}
		intconv.PutUint32(buf[i+1:], math.Float32bits(v))
		i += 5
	}
 {{- end}}
{{else if eq .Type "float64"}}
 {{- if .TypeList}}
	if l := len(o.{{.NameTitle}}); l != 0 {
		buf[i] = {{.Index}}
		i++
		x := uint(l)
		for x >= 0x80 {
			buf[i] = byte(x | 0x80)
			x >>= 7
			i++
		}
		buf[i] = byte(x)
		i++
		for _, v := range o.{{.NameTitle}} {
			intconv.PutUint64(buf[i:], math.Float64bits(v))
			i += 8
		}
	}
 {{- else}}
	if v := o.{{.NameTitle}}; v != 0 {
		buf[i] = {{.Index}}
		intconv.PutUint64(buf[i+1:], math.Float64bits(v))
		i += 9
	}
 {{- end}}
{{else if eq .Type "timestamp"}}
	if v := o.{{.NameTitle}}; !v.IsZero() {
		s, ns := uint64(v.Unix()), uint32(v.Nanosecond())
		if s < 1<<32 {
			buf[i] = {{.Index}}
			intconv.PutUint32(buf[i+1:], uint32(s))
			i += 5
		} else {
			buf[i] = {{.Index}} | 0x80
			intconv.PutUint64(buf[i+1:], s)
			i += 9
		}
		intconv.PutUint32(buf[i:], ns)
		i += 4
	}
{{else if eq .Type "text" "binary"}}
	if l := len(o.{{.NameTitle}}); l != 0 {
		buf[i] = {{.Index}}
		i++
		x := uint(l)
		for x >= 0x80 {
			buf[i] = byte(x | 0x80)
			x >>= 7
			i++
		}
		buf[i] = byte(x)
		i++
 {{- if .TypeList}}
		for _, a := range o.{{.NameTitle}} {
			x = uint(len(a))
			for x >= 0x80 {
				buf[i] = byte(x | 0x80)
				x >>= 7
				i++
			}
			buf[i] = byte(x)
			i++
			i += copy(buf[i:], a)
		}
 {{- else}}
		i += copy(buf[i:], o.{{.NameTitle}})
 {{- end}}
	}
{{else if .TypeList}}
	if l := len(o.{{.NameTitle}}); l != 0 {
		buf[i] = {{.Index}}
		i++
		x := uint(l)
		for x >= 0x80 {
			buf[i] = byte(x | 0x80)
			x >>= 7
			i++
		}
		buf[i] = byte(x)
		i++
		for vi, v := range o.{{.NameTitle}} {
			if v == nil {
				v = new({{.TypeNative}})
				o.{{.NameTitle}}[vi] = v
			}
			i += v.MarshalTo(buf[i:])
		}
	}
{{else}}
	if v := o.{{.NameTitle}}; v != nil {
		buf[i] = {{.Index}}
		i++
		i += v.MarshalTo(buf[i:])
	}
{{end}}`

const goMarshalFieldLen = `{{if eq .Type "bool"}}
	if o.{{.NameTitle}} {
		l++
	}
{{else if eq .Type "uint8"}}
	if x := o.{{.NameTitle}}; x != 0 {
		l += 2
	}
{{else if eq .Type "uint16"}}
	if x := o.{{.NameTitle}}; x >= 1<<8 {
		l += 3
	} else if x != 0 {
		l += 2
	}
{{else if eq .Type "uint32"}}
	if x := o.{{.NameTitle}}; x >= 1<<21 {
		l += 5
	} else if x != 0 {
		l += 2
		for x >= 0x80 {
			x >>= 7
			l++
		}
	}
{{else if eq .Type "uint64"}}
	if x := o.{{.NameTitle}}; x >= 1<<49 {
		l += 9
	} else if x != 0 {
		l += 2
		for x >= 0x80 {
			x >>= 7
			l++
		}
	}
{{else if eq .Type "int32"}}
	if v := o.{{.NameTitle}}; v != 0 {
		l += 2
		x := uint32(v)
		if v < 0 {
			x = ^x + 1
		}
		for x >= 0x80 {
			x >>= 7
			l++
		}
	}
{{else if eq .Type "int64"}}
	if v := o.{{.NameTitle}}; v != 0 {
		l += 2
		x := uint64(v)
		if v < 0 {
			x = ^x + 1
		}
		for n := 0; x >= 0x80 && n < 8; n++ {
			x >>= 7
			l++
		}
	}
{{else if eq .Type "float32"}}
 {{- if .TypeList}}
	if x := len(o.{{.NameTitle}}); x != 0 {
		l += 2 + x*4
		for x >= 0x80 {
			x >>= 7
			l++
		}
	}
 {{- else}}
	if o.{{.NameTitle}} != 0 {
		l += 5
	}
 {{- end}}
{{else if eq .Type "float64"}}
 {{- if .TypeList}}
	if x := len(o.{{.NameTitle}}); x != 0 {
		l += 2 + x*8
		for x >= 0x80 {
			x >>= 7
			l++
		}
	}
 {{- else}}
	if o.{{.NameTitle}} != 0 {
		l += 9
	}
 {{- end}}
{{else if eq .Type "timestamp"}}
	if v := o.{{.NameTitle}}; !v.IsZero() {
		if s := uint64(v.Unix()); s < 1<<32 {
			l += 9
		} else {
			l += 13
		}
	}
{{else if eq .Type "text" "binary"}}
	if x := len(o.{{.NameTitle}}); x != 0 {
 {{- if .TypeList}}
		if x > ColferListMax {
			return -1, ColferMax(fmt.Sprintf("colfer: field {{.String}} exceeds %d elements", ColferListMax))
		}
		for x >= 0x80 {
			x >>= 7
			l++
		}
		l += 2
		for _, a := range o.{{.NameTitle}} {
			x = len(a)
			l += x
			for x >= 0x80 {
				x >>= 7
				l++
			}
			l++
		}
 {{- else}}
		l += x
		for x >= 0x80 {
			x >>= 7
			l++
		}
		l += 2
 {{- end}}
	}
{{else if .TypeList}}
	if x := len(o.{{.NameTitle}}); x != 0 {
		if x > ColferListMax {
			return -1, ColferMax(fmt.Sprintf("colfer: field {{.String}} exceeds %d elements", ColferListMax))
		}
		for x >= 0x80 {
			x >>= 7
			l++
		}
		l += 2
		for _, v := range o.{{.NameTitle}} {
			if v == nil {
				l++
				continue
			}
			vl, err := v.MarshalLen()
			if err != nil {
				return -1, err
			}
			l += vl
		}
	}
{{else}}
	if v := o.{{.NameTitle}}; v != nil {
		vl, err := v.MarshalLen()
		if err != nil {
			return -1, err
		}
		l += vl + 1
	}
{{end}}`

const goUnmarshalField = `{{if eq .Type "bool"}}
	if header == {{.Index}} {
		if i >= len(data) {
			goto eof
		}
		o.{{.NameTitle}} = true
		header = data[i]
		i++
	}
{{else if eq .Type "uint8"}}
	if header == {{.Index}} {
		start := i
		i++
		if i >= len(data) {
			goto eof
		}
		o.{{.NameTitle}} = data[start]
		header = data[i]
		i++
	}
{{else if eq .Type "uint16"}}
	if header == {{.Index}} {
		start := i
		i += 2
		if i >= len(data) {
			goto eof
		}
		o.{{.NameTitle}} = intconv.Uint16(data[start:])
		header = data[i]
		i++
	} else if header == {{.Index}}|0x80 {
		start := i
		i++
		if i >= len(data) {
			goto eof
		}
		o.{{.NameTitle}} = uint16(data[start])
		header = data[i]
		i++
	}
{{else if eq .Type "uint32"}}
	if header == {{.Index}} {
		start := i
		i++
		if i >= len(data) {
			goto eof
		}
		x := uint32(data[start])

		if x >= 0x80 {
			x &= 0x7f
			for shift := uint(7); ; shift += 7 {
				b := uint32(data[i])
				i++
				if i >= len(data) {
					goto eof
				}

				if b < 0x80 {
					x |= b << shift
					break
				}
				x |= (b & 0x7f) << shift
			}
		}
		o.{{.NameTitle}} = x

		header = data[i]
		i++
	} else if header == {{.Index}}|0x80 {
		start := i
		i += 4
		if i >= len(data) {
			goto eof
		}
		o.{{.NameTitle}} = intconv.Uint32(data[start:])
		header = data[i]
		i++
	}
{{else if eq .Type "uint64"}}
	if header == {{.Index}} {
		start := i
		i++
		if i >= len(data) {
			goto eof
		}
		x := uint64(data[start])

		if x >= 0x80 {
			x &= 0x7f
			for shift := uint(7); ; shift += 7 {
				b := uint64(data[i])
				i++
				if i >= len(data) {
					goto eof
				}

				if b < 0x80 || shift == 56 {
					x |= b << shift
					break
				}
				x |= (b & 0x7f) << shift
			}
		}
		o.{{.NameTitle}} = x

		header = data[i]
		i++
	} else if header == {{.Index}}|0x80 {
		start := i
		i += 8
		if i >= len(data) {
			goto eof
		}
		o.{{.NameTitle}} = intconv.Uint64(data[start:])
		header = data[i]
		i++
	}
{{else if eq .Type "int32"}}
	if header == {{.Index}} {
		if i+1 >= len(data) {
			i++
			goto eof
		}
		x := uint32(data[i])
		i++

		if x >= 0x80 {
			x &= 0x7f
			for shift := uint(7); ; shift += 7 {
				b := uint32(data[i])
				i++
				if i >= len(data) {
					goto eof
				}

				if b < 0x80 {
					x |= b << shift
					break
				}
				x |= (b & 0x7f) << shift
			}
		}
		o.{{.NameTitle}} = int32(x)

		header = data[i]
		i++
	} else if header == {{.Index}}|0x80 {
		if i+1 >= len(data) {
			i++
			goto eof
		}
		x := uint32(data[i])
		i++

		if x >= 0x80 {
			x &= 0x7f
			for shift := uint(7); ; shift += 7 {
				b := uint32(data[i])
				i++
				if i >= len(data) {
					goto eof
				}

				if b < 0x80 {
					x |= b << shift
					break
				}
				x |= (b & 0x7f) << shift
			}
		}
		o.{{.NameTitle}} = int32(^x + 1)

		header = data[i]
		i++
	}
{{else if eq .Type "int64"}}
	if header == {{.Index}} {
		if i+1 >= len(data) {
			i++
			goto eof
		}
		x := uint64(data[i])
		i++

		if x >= 0x80 {
			x &= 0x7f
			for shift := uint(7); ; shift += 7 {
				b := uint64(data[i])
				i++
				if i >= len(data) {
					goto eof
				}

				if b < 0x80 || shift == 56 {
					x |= b << shift
					break
				}
				x |= (b & 0x7f) << shift
			}
		}
		o.{{.NameTitle}} = int64(x)

		header = data[i]
		i++
	} else if header == {{.Index}}|0x80 {
		if i+1 >= len(data) {
			i++
			goto eof
		}
		x := uint64(data[i])
		i++

		if x >= 0x80 {
			x &= 0x7f
			for shift := uint(7); ; shift += 7 {
				b := uint64(data[i])
				i++
				if i >= len(data) {
					goto eof
				}

				if b < 0x80 || shift == 56 {
					x |= b << shift
					break
				}
				x |= (b & 0x7f) << shift
			}
		}
		o.{{.NameTitle}} = int64(^x + 1)

		header = data[i]
		i++
	}
{{else if eq .Type "float32"}}
 {{- if .TypeList}}
	if header == {{.Index}} {
{{template "unmarshal-varint" .}}
		if x > uint(ColferListMax) {
			return 0, ColferMax(fmt.Sprintf("colfer: {{.String}} length %d exceeds %d elements", x, ColferListMax))
		}

		l := int(x)

		if end := i + l*4; end >= len(data) {
			i = end
			goto eof
		}
		a := make([]float32, l)
		for ai := range a {
			a[ai] = math.Float32frombits(intconv.Uint32(data[i:]))
			i += 4
		}
		o.{{.NameTitle}} = a

		header = data[i]
		i++
	}
 {{- else}}
	if header == {{.Index}} {
		start := i
		i += 4
		if i >= len(data) {
			goto eof
		}
		o.{{.NameTitle}} = math.Float32frombits(intconv.Uint32(data[start:]))
		header = data[i]
		i++
	}
 {{- end}}
{{else if eq .Type "float64"}}
 {{- if .TypeList}}
	if header == {{.Index}} {
{{template "unmarshal-varint" .}}
		if x > uint(ColferListMax) {
			return 0, ColferMax(fmt.Sprintf("colfer: {{.String}} length %d exceeds %d elements", x, ColferListMax))
		}
		l := int(x)

		if end := i + l*8; end >= len(data) {
			i = end
			goto eof
		}
		a := make([]float64, l)
		for ai := range a {
			a[ai] = math.Float64frombits(intconv.Uint64(data[i:]))
			i += 8
		}
		o.{{.NameTitle}} = a

		header = data[i]
		i++
	}
 {{- else}}
	if header == {{.Index}} {
		start := i
		i += 8
		if i >= len(data) {
			goto eof
		}
		o.{{.NameTitle}} = math.Float64frombits(intconv.Uint64(data[start:]))
		header = data[i]
		i++
	}
 {{- end}}
{{else if eq .Type "timestamp"}}
	if header == {{.Index}} {
		start := i
		i += 8
		if i >= len(data) {
			goto eof
		}
		o.{{.NameTitle}} = time.Unix(int64(intconv.Uint32(data[start:])), int64(intconv.Uint32(data[start+4:]))).In(time.UTC)
		header = data[i]
		i++
	} else if header == {{.Index}}|0x80 {
		start := i
		i += 12
		if i >= len(data) {
			goto eof
		}
		o.{{.NameTitle}} = time.Unix(int64(intconv.Uint64(data[start:])), int64(intconv.Uint32(data[start+8:]))).In(time.UTC)
		header = data[i]
		i++
	}
{{else if eq .Type "text"}}
	if header == {{.Index}} {
{{template "unmarshal-varint" .}}
 {{- if .TypeList}}
		if x > uint(ColferListMax) {
			return 0, ColferMax(fmt.Sprintf("colfer: {{.String}} length %d exceeds %d elements", x, ColferListMax))
		}
		a := make([]string, int(x))
		o.{{.NameTitle}} = a

		for ai := range a {
{{template "unmarshal-varint" .}}
			if x > uint(ColferSizeMax) {
				return 0, ColferMax(fmt.Sprintf("colfer: {{.String}} element %d size %d exceeds %d bytes", ai, x, ColferSizeMax))
			}

			start := i
			i += int(x)
			if i >= len(data) {
				goto eof
			}
			a[ai] = string(data[start:i])
		}

		if i >= len(data) {
			goto eof
		}
		header = data[i]
		i++
	}
 {{- else}}
		if x > uint(ColferSizeMax) {
			return 0, ColferMax(fmt.Sprintf("colfer: {{.String}} size %d exceeds %d bytes", x, ColferSizeMax))
		}

		start := i
		i += int(x)
		if i >= len(data) {
			goto eof
		}
		o.{{.NameTitle}} = string(data[start:i])

		header = data[i]
		i++
	}
 {{- end}}
{{else if eq .Type "binary"}}
	if header == {{.Index}} {
{{template "unmarshal-varint" .}}
 {{- if not .TypeList}}
		if x > uint(ColferSizeMax) {
			return 0, ColferMax(fmt.Sprintf("colfer: {{.String}} size %d exceeds %d bytes", x, ColferSizeMax))
		}
		v := make([]byte, int(x))

		start := i
		i += len(v)
		if i >= len(data) {
			goto eof
		}
		copy(v, data[start:i])
		o.{{.NameTitle}} = v

		header = data[i]
		i++
 {{- else}}
		if x > uint(ColferListMax) {
			return 0, ColferMax(fmt.Sprintf("colfer: {{.String}} length %d exceeds %d elements", x, ColferListMax))
		}
		a := make([][]byte, int(x))
		o.{{.NameTitle}} = a
		for ai := range a {
{{template "unmarshal-varint" .}}
			if x > uint(ColferSizeMax) {
				return 0, ColferMax(fmt.Sprintf("colfer: {{.String}} element %d size %d exceeds %d bytes", ai, x, ColferSizeMax))
			}
			v := make([]byte, int(x))

			start := i
			i += len(v)
			if i >= len(data) {
				goto eof
			}

			copy(v, data[start:i])
			a[ai] = v
		}

		if i >= len(data) {
			goto eof
		}
		header = data[i]
		i++
 {{- end}}
	}
{{else if .TypeList}}
	if header == {{.Index}} {
{{template "unmarshal-varint" .}}
		if x > uint(ColferListMax) {
			return 0, ColferMax(fmt.Sprintf("colfer: {{.String}} length %d exceeds %d elements", x, ColferListMax))
		}

		l := int(x)
		a := make([]*{{.TypeNative}}, l)
		malloc := make([]{{.TypeNative}}, l)
		for ai, _ := range a {
			v := &malloc[ai]
			a[ai] = v

			n, err := v.Unmarshal(data[i:])
			if err != nil {
				if err == io.EOF && len(data) >= ColferSizeMax {
					return 0, ColferMax(fmt.Sprintf("colfer: {{.Struct.String}} size exceeds %d bytes", ColferSizeMax))
				}
				return 0, err
			}
			i += n
		}
		o.{{.NameTitle}} = a

		if i >= len(data) {
			goto eof
		}
		header = data[i]
		i++
	}
{{else}}
	if header == {{.Index}} {
		o.{{.NameTitle}} = new({{.TypeNative}})
		n, err := o.{{.NameTitle}}.Unmarshal(data[i:])
		if err != nil {
			if err == io.EOF && len(data) >= ColferSizeMax {
				return 0, ColferMax(fmt.Sprintf("colfer: {{.Struct.String}} size exceeds %d bytes", ColferSizeMax))
			}
			return 0, err
		}
		i += n

		if i >= len(data) {
			goto eof
		}
		header = data[i]
		i++
	}
{{end}}`

const goUnmarshalVarint = `		if i >= len(data) {
			goto eof
		}
		x := uint(data[i])
		i++

		if x >= 0x80 {
			x &= 0x7f
			for shift := uint(7); ; shift += 7 {
				if i >= len(data) {
					goto eof
				}
				b := uint(data[i])
				i++

				if b < 0x80 {
					x |= b << shift
					break
				}
				x |= (b & 0x7f) << shift
			}
		}
`