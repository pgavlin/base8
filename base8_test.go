package base8

import (
	"bytes"
	"io"
	"io/ioutil"
	"strings"
	"testing"
)

type testpair struct {
	decoded, encoded string
}

var pairs = []testpair{
	// RFC 4648 examples
	{"", ""},
	{"f", "314====="},
	{"fo", "314674=="},
	{"foo", "31467557"},
	{"foob", "31467557304====="},
	{"fooba", "31467557304604=="},
	{"foobar", "3146755730460562"},

	// Wikipedia examples, converted to base8
	{"sure.", "34672562312270=="},
	{"sure", "34672562312====="},
	{"sur", "34672562"},
	{"su", "346724=="},
	{"leasure.", "3306254134672562312270=="},
	{"easure.", "3126056335271145134====="},
	{"asure.", "3027156534462456"},
	{"sure.", "34672562312270=="},
}

var bigtest = testpair{
	"Twas brillig, and the slithy toves",
	"2507354134620142344645543306454713020141334620403506414510071554322721503622016433673145346=====",
}

func testEqual(t *testing.T, msg string, args ...interface{}) bool {
	t.Helper()
	if args[len(args)-2] != args[len(args)-1] {
		t.Errorf(msg, args...)
		return false
	}
	return true
}

func TestEncode(t *testing.T) {
	for _, p := range pairs {
		got := EncodeToString([]byte(p.decoded))
		testEqual(t, "Encode(%q) = %q, want %q", p.decoded, got, p.encoded)
	}
}

func TestEncoder(t *testing.T) {
	for _, p := range pairs {
		bb := &bytes.Buffer{}
		encoder := NewEncoder(bb)
		encoder.Write([]byte(p.decoded))
		encoder.Close()
		testEqual(t, "Encode(%q) = %q, want %q", p.decoded, bb.String(), p.encoded)
	}
}

func TestEncoderBuffering(t *testing.T) {
	input := []byte(bigtest.decoded)
	for bs := 1; bs <= 12; bs++ {
		bb := &bytes.Buffer{}
		encoder := NewEncoder(bb)
		for pos := 0; pos < len(input); pos += bs {
			end := pos + bs
			if end > len(input) {
				end = len(input)
			}
			n, err := encoder.Write(input[pos:end])
			testEqual(t, "Write(%q) gave error %v, want %v", input[pos:end], err, error(nil))
			testEqual(t, "Write(%q) gave length %v, want %v", input[pos:end], n, end-pos)
		}
		err := encoder.Close()
		testEqual(t, "Close gave error %v, want %v", err, error(nil))
		testEqual(t, "Encoding/%d of %q = %q, want %q", bs, bigtest.decoded, bb.String(), bigtest.encoded)
	}
}

func TestDecode(t *testing.T) {
	for _, p := range pairs {
		dbuf := make([]byte, DecodedLen(len(p.encoded)))
		count, end, err := decode(dbuf, []byte(p.encoded))
		testEqual(t, "Decode(%q) = error %v, want %v", p.encoded, err, error(nil))
		testEqual(t, "Decode(%q) = length %v, want %v", p.encoded, count, len(p.decoded))
		if len(p.encoded) > 0 {
			testEqual(t, "Decode(%q) = end %v, want %v", p.encoded, end, (p.encoded[len(p.encoded)-1] == '='))
		}
		testEqual(t, "Decode(%q) = %q, want %q", p.encoded,
			string(dbuf[0:count]),
			p.decoded)

		dbuf, err = DecodeString(p.encoded)
		testEqual(t, "DecodeString(%q) = error %v, want %v", p.encoded, err, error(nil))
		testEqual(t, "DecodeString(%q) = %q, want %q", p.encoded, string(dbuf), p.decoded)
	}
}

func TestDecoder(t *testing.T) {
	for _, p := range pairs {
		decoder := NewDecoder(strings.NewReader(p.encoded))
		dbuf := make([]byte, DecodedLen(len(p.encoded)))
		count, err := decoder.Read(dbuf)
		if err != nil && err != io.EOF {
			t.Fatal("Read failed", err)
		}
		testEqual(t, "Read from %q = length %v, want %v", p.encoded, count, len(p.decoded))
		testEqual(t, "Decoding of %q = %q, want %q", p.encoded, string(dbuf[0:count]), p.decoded)
		if err != io.EOF {
			_, err = decoder.Read(dbuf)
		}
		testEqual(t, "Read from %q = %v, want %v", p.encoded, err, io.EOF)
	}
}

type badReader struct {
	data   []byte
	errs   []error
	called int
	limit  int
}

// Populates p with data, returns a count of the bytes written and an
// error.  The error returned is taken from badReader.errs, with each
// invocation of Read returning the next error in this slice, or io.EOF,
// if all errors from the slice have already been returned.  The
// number of bytes returned is determined by the size of the input buffer
// the test passes to decoder.Read and will be a multiple of 8, unless
// badReader.limit is non zero.
func (b *badReader) Read(p []byte) (int, error) {
	lim := len(p)
	if b.limit != 0 && b.limit < lim {
		lim = b.limit
	}
	if len(b.data) < lim {
		lim = len(b.data)
	}
	for i := range p[:lim] {
		p[i] = b.data[i]
	}
	b.data = b.data[lim:]
	err := io.EOF
	if b.called < len(b.errs) {
		err = b.errs[b.called]
	}
	b.called++
	return lim, err
}

// TestDecoderError verifies decode errors are propagated when there are no read
// errors.
func TestDecoderError(t *testing.T) {
	for _, readErr := range []error{io.EOF, nil} {
		input := "01234568"
		dbuf := make([]byte, DecodedLen(len(input)))
		br := badReader{data: []byte(input), errs: []error{readErr}}
		decoder := NewDecoder(&br)
		n, err := decoder.Read(dbuf)
		testEqual(t, "Read after EOF, n = %d, expected %d", n, 0)
		if _, ok := err.(CorruptInputError); !ok {
			t.Errorf("Corrupt input error expected.  Found %T (%v)", err, err)
		}
	}
}

// TestReaderEOF ensures decoder.Read behaves correctly when input data is
// exhausted.
func TestReaderEOF(t *testing.T) {
	for _, readErr := range []error{io.EOF, nil} {
		input := "01234567"
		br := badReader{data: []byte(input), errs: []error{nil, readErr}}
		decoder := NewDecoder(&br)
		dbuf := make([]byte, DecodedLen(len(input)))
		n, err := decoder.Read(dbuf)
		testEqual(t, "Decoding of %q err = %v, expected %v", string(input), err, error(nil))
		n, err = decoder.Read(dbuf)
		testEqual(t, "Read after EOF, n = %d, expected %d", n, 0)
		testEqual(t, "Read after EOF, err = %v, expected %v", err, io.EOF)
		n, err = decoder.Read(dbuf)
		testEqual(t, "Read after EOF, n = %d, expected %d", n, 0)
		testEqual(t, "Read after EOF, err = %v, expected %v", err, io.EOF)
	}
}

func TestDecoderBuffering(t *testing.T) {
	for bs := 1; bs <= 12; bs++ {
		decoder := NewDecoder(strings.NewReader(bigtest.encoded))
		buf := make([]byte, len(bigtest.decoded)+12)
		var total int
		var n int
		var err error
		for total = 0; total < len(bigtest.decoded) && err == nil; {
			n, err = decoder.Read(buf[total : total+bs])
			total += n
		}
		if err != nil && err != io.EOF {
			t.Errorf("Read from %q at pos %d = %d, unexpected error %v", bigtest.encoded, total, n, err)
		}
		testEqual(t, "Decoding/%d of %q = %q, want %q", bs, bigtest.encoded, string(buf[0:total]), bigtest.decoded)
	}
}

func TestDecodeCorrupt(t *testing.T) {
	testCases := []struct {
		input  string
		offset int // -1 means no corruption.
	}{
		{"", -1},
		{"!!!!", 0},
		{"x===", 0},
		{"11=1====", 2},
		{"111=1111", 3},
		{"222222222", 8},
		{"222222", 0},
		{"1=", 1},
		{"11=", 3},
		{"11==", 4},
		{"11===", 5},
		{"1111=", 5},
		{"1111==", 6},
		{"11111=", 6},
		{"11111==", 7},
		{"1=======", 1},
		{"11======", 2},
		{"111=====", -1},
		{"1111====", 4},
		{"11111===", 5},
		{"111111==", -1},
		{"1111111=", 7},
		{"11111111", -1},
	}
	for _, tc := range testCases {
		dbuf := make([]byte, DecodedLen(len(tc.input)))
		_, err := Decode(dbuf, []byte(tc.input))
		if tc.offset == -1 {
			if err != nil {
				t.Error("Decoder wrongly detected corruption in", tc.input)
			}
			continue
		}
		switch err := err.(type) {
		case CorruptInputError:
			testEqual(t, "Corruption in %q at offset %v, want %v", tc.input, int(err), tc.offset)
		default:
			t.Error("Decoder failed to detect corruption in", tc)
		}
	}
}

func TestBig(t *testing.T) {
	n := 3*1000 + 1
	raw := make([]byte, n)
	const alpha = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	for i := 0; i < n; i++ {
		raw[i] = alpha[i%len(alpha)]
	}
	encoded := new(bytes.Buffer)
	w := NewEncoder(encoded)
	nn, err := w.Write(raw)
	if nn != n || err != nil {
		t.Fatalf("Encoder.Write(raw) = %d, %v want %d, nil", nn, err, n)
	}
	err = w.Close()
	if err != nil {
		t.Fatalf("Encoder.Close() = %v want nil", err)
	}
	decoded, err := ioutil.ReadAll(NewDecoder(encoded))
	if err != nil {
		t.Fatalf("ioutil.ReadAll(NewDecoder(...)): %v", err)
	}

	if !bytes.Equal(raw, decoded) {
		var i int
		for i = 0; i < len(decoded) && i < len(raw); i++ {
			if decoded[i] != raw[i] {
				break
			}
		}
		t.Errorf("Decode(Encode(%d-byte string)) failed at offset %d", n, i)
	}
}

func testStringEncoding(t *testing.T, expected string, examples []string) {
	for _, e := range examples {
		buf, err := DecodeString(e)
		if err != nil {
			t.Errorf("Decode(%q) failed: %v", e, err)
			continue
		}
		if s := string(buf); s != expected {
			t.Errorf("Decode(%q) = %q, want %q", e, s, expected)
		}
	}
}

func BenchmarkEncode(b *testing.B) {
	data := make([]byte, 8192)
	buf := make([]byte, EncodedLen(len(data)))
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		Encode(buf, data)
	}
}

func BenchmarkEncodeToString(b *testing.B) {
	data := make([]byte, 8192)
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		EncodeToString(data)
	}
}

func BenchmarkDecode(b *testing.B) {
	data := make([]byte, EncodedLen(8192))
	Encode(data, make([]byte, 8192))
	buf := make([]byte, 8192)
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		Decode(buf, data)
	}
}
func BenchmarkDecodeString(b *testing.B) {
	data := EncodeToString(make([]byte, 8192))
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		DecodeString(data)
	}
}

func TestDecodeWithPadding(t *testing.T) {
	for _, pair := range pairs {

		input := pair.decoded
		encoded := EncodeToString([]byte(input))

		decoded, err := DecodeString(encoded)
		if err != nil {
			t.Errorf("DecodeString Error (%q): %v", input, err)
		}

		if input != string(decoded) {
			t.Errorf("Unexpected result: got %q; want %q", decoded, input)
		}
	}
}

func TestEncodedDecodedLen(t *testing.T) {
	type test struct {
		in      int
		wantEnc int
		wantDec int
	}
	data := bytes.Repeat([]byte("x"), 100)
	for _, tc := range []test{
		{0, 0, 0},
		{1, 8, 3},
		{5, 16, 6},
		{6, 16, 6},
		{10, 32, 12},
	} {
		encLen := EncodedLen(tc.in)
		decLen := DecodedLen(encLen)
		enc := EncodeToString(data[:tc.in])
		if len(enc) != encLen {
			t.Fatalf("EncodedLen(%d) = %d but encoded to %q (%d)", tc.in, encLen, enc, len(enc))
		}
		if encLen != tc.wantEnc {
			t.Fatalf("EncodedLen(%d) = %d; want %d", tc.in, encLen, tc.wantEnc)
		}
		if decLen != tc.wantDec {
			t.Fatalf("DecodedLen(%d) = %d; want %d", encLen, decLen, tc.wantDec)
		}
	}
}

func TestWithoutPaddingClose(t *testing.T) {
	for _, testpair := range pairs {

		var buf bytes.Buffer
		encoder := NewEncoder(&buf)
		encoder.Write([]byte(testpair.decoded))
		encoder.Close()

		expected := testpair.encoded
		res := buf.String()

		if res != expected {
			t.Errorf("Expected %s got %s", expected, res)
		}
	}
}

func TestDecodeReadAll(t *testing.T) {
	for _, pair := range pairs {
		encoded := pair.encoded
		decReader, err := ioutil.ReadAll(NewDecoder(strings.NewReader(encoded)))
		if err != nil {
			t.Errorf("NewDecoder error: %v", err)
		}

		if pair.decoded != string(decReader) {
			t.Errorf("Expected %s got %s", pair.decoded, decReader)
		}
	}
}

func TestDecodeSmallBuffer(t *testing.T) {
	for bufferSize := 1; bufferSize < 200; bufferSize++ {
		for _, pair := range pairs {
			encoded := pair.encoded
			decoder := NewDecoder(strings.NewReader(encoded))

			var allRead []byte

			for {
				buf := make([]byte, bufferSize)
				n, err := decoder.Read(buf)
				allRead = append(allRead, buf[0:n]...)
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Error(err)
				}
			}

			if pair.decoded != string(allRead) {
				t.Errorf("Expected %s got %s; bufferSize %d", pair.decoded, allRead, bufferSize)
			}
		}
	}
}
