// Package base8 implements base8 encoding.
package base8

import (
	"io"
	"strconv"
)

/*
 * Encoder
 */

const encodeTable = "01234567"
const PadChar = '='

// Encode encodes src using the encoding enc, writing
// EncodedLen(len(src)) bytes to dst.
//
// The encoding pads the output to a multiple of 8 bytes,
// so Encode is not appropriate for use on individual blocks
// of a large data stream. Use NewEncoder() instead.
func Encode(dst, src []byte) {
	for len(src) > 0 {
		var b [8]byte

		// Unpack 8x 3-bit source blocks into an 8 byte
		// destination quantum
		switch len(src) {
		default:
			b[7] = src[2] & 0x7        // bits [0:2]
			b[6] = (src[2] >> 3) & 0x7 // bits [3:5]
			b[5] = src[2] >> 6         // bits [6:7]
			fallthrough
		case 2:
			b[5] |= (src[1] << 2) & 0x7 // bits [0:0]
			b[4] = (src[1] >> 1) & 0x7  // bits [1:3]
			b[3] = (src[1] >> 4) & 0x7  // bits [4:6]
			b[2] = src[1] >> 7          // bits [7:7]
			fallthrough
		case 1:
			b[2] |= (src[0] << 1) & 0x7 // bits [0:1]
			b[1] = (src[0] >> 2) & 0x7  // bits [2:4]
			b[0] = src[0] >> 5          // bits [5:7]
		}

		// Encode 3-bit blocks using the base8 alphabet
		size := len(dst)
		if size >= 8 {
			// Common case, unrolled for extra performance
			dst[0] = encodeTable[b[0]&7]
			dst[1] = encodeTable[b[1]&7]
			dst[2] = encodeTable[b[2]&7]
			dst[3] = encodeTable[b[3]&7]
			dst[4] = encodeTable[b[4]&7]
			dst[5] = encodeTable[b[5]&7]
			dst[6] = encodeTable[b[6]&7]
			dst[7] = encodeTable[b[7]&7]
		} else {
			for i := 0; i < size; i++ {
				dst[i] = encodeTable[b[i]&7]
			}
		}

		// Pad the final quantum
		if len(src) < 3 {
			dst[7] = PadChar
			dst[6] = PadChar
			if len(src) < 2 {
				dst[5] = PadChar
				dst[4] = PadChar
				dst[3] = PadChar
			}

			break
		}

		src = src[3:]
		dst = dst[8:]
	}
}

// EncodeToString returns the base8 encoding of src.
func EncodeToString(src []byte) string {
	buf := make([]byte, EncodedLen(len(src)))
	Encode(buf, src)
	return string(buf)
}

type encoder struct {
	err  error
	w    io.Writer
	buf  [3]byte    // buffered data waiting to be encoded
	nbuf int        // number of bytes in buf
	out  [1024]byte // output buffer
}

func (e *encoder) Write(p []byte) (n int, err error) {
	if e.err != nil {
		return 0, e.err
	}

	// Leading fringe.
	if e.nbuf > 0 {
		var i int
		for i = 0; i < len(p) && e.nbuf < 3; i++ {
			e.buf[e.nbuf] = p[i]
			e.nbuf++
		}
		n += i
		p = p[i:]
		if e.nbuf < 3 {
			return
		}
		Encode(e.out[0:], e.buf[0:])
		if _, e.err = e.w.Write(e.out[0:8]); e.err != nil {
			return n, e.err
		}
		e.nbuf = 0
	}

	// Large interior chunks.
	for len(p) >= 3 {
		nn := len(e.out) / 8 * 3
		if nn > len(p) {
			nn = len(p)
			nn -= nn % 3
		}
		Encode(e.out[0:], p[0:nn])
		if _, e.err = e.w.Write(e.out[0 : nn/3*8]); e.err != nil {
			return n, e.err
		}
		n += nn
		p = p[nn:]
	}

	// Trailing fringe.
	for i := 0; i < len(p); i++ {
		e.buf[i] = p[i]
	}
	e.nbuf = len(p)
	n += len(p)
	return
}

// Close flushes any pending output from the encoder.
// It is an error to call Write after calling Close.
func (e *encoder) Close() error {
	// If there's anything left in the buffer, flush it out
	if e.err == nil && e.nbuf > 0 {
		Encode(e.out[0:], e.buf[0:e.nbuf])
		encodedLen := EncodedLen(e.nbuf)
		e.nbuf = 0
		_, e.err = e.w.Write(e.out[0:encodedLen])
	}
	return e.err
}

// NewEncoder returns a new base8 stream encoder. Data written to
// the returned writer will be encoded using enc and then written to w.
// Base8 operates in 3-byte blocks; when finished writing, the caller
// must Close the returned encoder to flush any partially written
// blocks.
func NewEncoder(w io.Writer) io.WriteCloser {
	return &encoder{w: w}
}

// EncodedLen returns the length in bytes of the base8 encoding
// of an input buffer of length n.
func EncodedLen(n int) int {
	return (n + 2) / 3 * 8
}

/*
 * Decoder
 */

type CorruptInputError int64

func (e CorruptInputError) Error() string {
	return "illegal base8 data at input byte " + strconv.FormatInt(int64(e), 10)
}

// decode is like Decode but returns an additional 'end' value, which
// indicates if end-of-message padding was encountered and thus any
// additional data is an error.
func decode(dst, src []byte) (n int, end bool, err error) {
	dsti := 0
	olen := len(src)

	for len(src) > 0 && !end {
		// Decode quantum using the base8 alphabet
		var dbuf [8]byte
		dlen := 8

		for j := 0; j < 8; {
			if len(src) == 0 {
				// We have reached the end and are missing padding
				return n, false, CorruptInputError(olen - len(src) - j)
			}
			in := src[0]
			src = src[1:]
			if in == byte(PadChar) && j >= 2 && len(src) < 8 {
				// We've reached the end and there's padding
				if len(src)+j < 8-1 {
					// not enough padding
					return n, false, CorruptInputError(olen)
				}
				for k := 0; k < 8-1-j; k++ {
					if len(src) > k && src[k] != byte(PadChar) {
						// incorrect padding
						return n, false, CorruptInputError(olen - len(src) + k - 1)
					}
				}
				dlen, end = j, true
				// 5 and 2 are the only valid padding lengths, so 3 and 6 are the only
				// valid dlen values.
				if dlen != 3 && dlen != 6 {
					return n, false, CorruptInputError(olen - len(src) - 1)
				}
				break
			}
			dbuf[j] = in - '0'
			if dbuf[j] > 7 {
				return n, false, CorruptInputError(olen - len(src) - 1)
			}
			j++
		}

		// Pack 8x 3-bit source blocks into 3 byte destination
		// quantum
		switch dlen {
		case 8:
			dst[dsti+2] = dbuf[5]<<6 | dbuf[6]<<3 | dbuf[7]
			n++
			fallthrough
		case 6:
			dst[dsti+1] = dbuf[2]<<7 | dbuf[3]<<4 | dbuf[4]<<1 | dbuf[5]>>2
			n++
			fallthrough
		case 3:
			dst[dsti] = dbuf[0]<<5 | dbuf[1]<<2 | dbuf[2]>>1
			n++
		}
		dsti += 3
	}
	return n, end, nil
}

// Decode decodes src using the encoding enc. It writes at most
// DecodedLen(len(src)) bytes to dst and returns the number of bytes
// written. If src contains invalid base8 data, it will return the
// number of bytes successfully written and CorruptInputError.
func Decode(dst, src []byte) (n int, err error) {
	n, _, err = decode(dst, src)
	return
}

// DecodeString returns the bytes represented by the base8 string s.
func DecodeString(s string) ([]byte, error) {
	buf := []byte(s)
	n, _, err := decode(buf, buf)
	return buf[:n], err
}

type decoder struct {
	err    error
	r      io.Reader
	end    bool       // saw end of message
	buf    [1024]byte // leftover input
	nbuf   int
	out    []byte // leftover decoded output
	outbuf [1024 / 8 * 3]byte
}

func readEncodedData(r io.Reader, buf []byte, min int) (n int, err error) {
	for n < min && err == nil {
		var nn int
		nn, err = r.Read(buf[n:])
		n += nn
	}
	// data was read, less than min bytes could be read
	if n < min && n > 0 && err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	// no data was read, the buffer already contains some data
	if min < 8 && n == 0 && err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return
}

func (d *decoder) Read(p []byte) (n int, err error) {
	// Use leftover decoded output from last read.
	if len(d.out) > 0 {
		n = copy(p, d.out)
		d.out = d.out[n:]
		if len(d.out) == 0 {
			return n, d.err
		}
		return n, nil
	}

	if d.err != nil {
		return 0, d.err
	}

	// Read a chunk.
	nn := len(p) / 3 * 8
	if nn < 8 {
		nn = 8
	}
	if nn > len(d.buf) {
		nn = len(d.buf)
	}

	// Minimum amount of bytes that needs to be read each cycle
	min := 8 - d.nbuf
	nn, d.err = readEncodedData(d.r, d.buf[d.nbuf:nn], min)
	d.nbuf += nn
	if d.nbuf < min {
		return 0, d.err
	}

	// Decode chunk into p, or d.out and then p if p is too small.
	nr := d.nbuf / 8 * 8
	nw := DecodedLen(d.nbuf)

	if nw > len(p) {
		nw, d.end, err = decode(d.outbuf[0:], d.buf[0:nr])
		d.out = d.outbuf[0:nw]
		n = copy(p, d.out)
		d.out = d.out[n:]
	} else {
		n, d.end, err = decode(p, d.buf[0:nr])
	}
	d.nbuf -= nr
	for i := 0; i < d.nbuf; i++ {
		d.buf[i] = d.buf[i+nr]
	}

	if err != nil && (d.err == nil || d.err == io.EOF) {
		d.err = err
	}

	if len(d.out) > 0 {
		// We cannot return all the decoded bytes to the caller in this
		// invocation of Read, so we return a nil error to ensure that Read
		// will be called again.  The error stored in d.err, if any, will be
		// returned with the last set of decoded bytes.
		return n, nil
	}

	return n, d.err
}

// NewDecoder constructs a new base32 stream decoder.
func NewDecoder(r io.Reader) io.Reader {
	return &decoder{r: r}
}

// DecodedLen returns the maximum length in bytes of the decoded data
// corresponding to n bytes of base32-encoded data.
func DecodedLen(n int) int {
	return n / 8 * 3
}
