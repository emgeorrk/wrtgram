package osslenc_test

import (
	"bytes"
	"encoding/base64"
	"testing"

	"github.com/emgeorrk/wrtgram/pkg/osslenc"
)

// Produced on macOS with:
//
//	printf '%s' 'correct horse' | openssl enc -aes-256-cbc -pbkdf2 -iter 200000 -salt -pass stdin -in plain.txt -out plain.enc
const (
	opensslVector = "U2FsdGVkX19xXx3vuf5nvjCGF4QLU2q7eAqkvrn70fsQh7lwZ1xWYsbbuOvnBpOtrctg+GRbj88yJwkpsbCIYA=="
	plainText     = "wrtgram backup fixture: hello, router!\n"
	password      = "correct horse"
)

func TestDecryptOpenSSLVector(t *testing.T) {
	t.Parallel()

	enc, err := base64.StdEncoding.DecodeString(opensslVector)
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := osslenc.Decrypt(&out, bytes.NewReader(enc), password, osslenc.DefaultIter); err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if out.String() != plainText {
		t.Errorf("Decrypt = %q, want %q", out.String(), plainText)
	}

	if err := osslenc.Decrypt(&bytes.Buffer{}, bytes.NewReader(enc), "wrong", osslenc.DefaultIter); err == nil {
		t.Error("wrong password must fail")
	}
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   []byte
	}{
		{name: "empty", in: []byte{}},
		{name: "block aligned", in: bytes.Repeat([]byte("0123456789abcdef"), 4)},
		{name: "odd length", in: []byte("tar.gz bytes \x00\x01\x02 here")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var enc, dec bytes.Buffer

			if err := osslenc.Encrypt(&enc, bytes.NewReader(tt.in), password, 1000); err != nil {
				t.Fatalf("Encrypt: %v", err)
			}

			if !bytes.HasPrefix(enc.Bytes(), []byte("Salted__")) {
				t.Error("missing Salted__ header")
			}

			if err := osslenc.Decrypt(&dec, bytes.NewReader(enc.Bytes()), password, 1000); err != nil {
				t.Fatalf("Decrypt: %v", err)
			}

			if !bytes.Equal(dec.Bytes(), tt.in) {
				t.Errorf("round trip mismatch")
			}
		})
	}
}
