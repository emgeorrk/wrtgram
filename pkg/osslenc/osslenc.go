// Package osslenc writes and reads the file format of
// `openssl enc -aes-256-cbc -pbkdf2 -salt`: the "Salted__" magic, an 8-byte
// salt, then AES-256-CBC with PKCS#7 padding, key and IV derived together
// with PBKDF2-HMAC-SHA256. Backups made by the bot therefore decrypt with
// stock openssl on any computer.
package osslenc

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// DefaultIter is the PBKDF2 iteration count the bot uses; pass it to
// `openssl enc -d -iter`.
const DefaultIter = 200000

const (
	magic   = "Salted__"
	saltLen = 8
	keyLen  = 32
	ivLen   = 16
	maxSize = 64 << 20
)

var (
	errMagic   = errors.New("not an openssl salted file")
	errPadding = errors.New("bad padding (wrong password?)")
	errTooBig  = errors.New("input too large")
)

// Encrypt reads src fully and writes the encrypted file to dst.
func Encrypt(dst io.Writer, src io.Reader, password string, iter int) error {
	plain, err := readAll(src)
	if err != nil {
		return err
	}

	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("salt: %w", err)
	}

	block, iv := derive(password, salt, iter)

	padded := pad(plain)
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(padded, padded)

	for _, part := range [][]byte{[]byte(magic), salt, padded} {
		if _, err := dst.Write(part); err != nil {
			return fmt.Errorf("write: %w", err)
		}
	}

	return nil
}

// Decrypt reverses Encrypt (used by tests and the self-check).
func Decrypt(dst io.Writer, src io.Reader, password string, iter int) error {
	data, err := readAll(src)
	if err != nil {
		return err
	}

	if len(data) < len(magic)+saltLen || string(data[:len(magic)]) != magic {
		return errMagic
	}

	salt := data[len(magic) : len(magic)+saltLen]
	body := data[len(magic)+saltLen:]

	if len(body) == 0 || len(body)%aes.BlockSize != 0 {
		return errPadding
	}

	block, iv := derive(password, salt, iter)
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(body, body)

	plain, err := unpad(body)
	if err != nil {
		return err
	}

	if _, err := dst.Write(plain); err != nil {
		return fmt.Errorf("write: %w", err)
	}

	return nil
}

func derive(password string, salt []byte, iter int) (block cipher.Block, iv []byte) {
	km := pbkdf2([]byte(password), salt, iter, keyLen+ivLen)

	block, err := aes.NewCipher(km[:keyLen])
	if err != nil {
		panic(err) // key length is constant and valid
	}

	return block, km[keyLen:]
}

// pbkdf2 is PBKDF2-HMAC-SHA256 (RFC 8018); the standard library gained it
// in Go 1.24, which the OpenWrt 24.10 SDK does not have.
func pbkdf2(password, salt []byte, iter, length int) []byte {
	prf := hmac.New(sha256.New, password)
	hashLen := prf.Size()
	out := make([]byte, 0, length)

	for block := uint32(1); len(out) < length; block++ {
		prf.Reset()
		prf.Write(salt)

		var idx [4]byte

		binary.BigEndian.PutUint32(idx[:], block)
		prf.Write(idx[:])

		u := prf.Sum(nil)
		t := make([]byte, hashLen)
		copy(t, u)

		for i := 1; i < iter; i++ {
			prf.Reset()
			prf.Write(u)
			u = prf.Sum(nil)

			for j := range t {
				t[j] ^= u[j]
			}
		}

		out = append(out, t...)
	}

	return out[:length]
}

func pad(b []byte) []byte {
	n := aes.BlockSize - len(b)%aes.BlockSize
	out := make([]byte, len(b)+n)
	copy(out, b)

	for i := len(b); i < len(out); i++ {
		out[i] = byte(n)
	}

	return out
}

func unpad(b []byte) ([]byte, error) {
	n := int(b[len(b)-1])
	if n == 0 || n > aes.BlockSize || n > len(b) {
		return nil, errPadding
	}

	for _, c := range b[len(b)-n:] {
		if int(c) != n {
			return nil, errPadding
		}
	}

	return b[:len(b)-n], nil
}

func readAll(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxSize+1))
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	if len(data) > maxSize {
		return nil, errTooBig
	}

	return data, nil
}
