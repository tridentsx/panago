package firmware

import (
	"crypto/aes"
	"crypto/cipher"
)

// Feistel cipher state
type feistelCipher struct {
	subkeys [32]byte
}

// NewFeistelCipher creates a new Feistel cipher with the default key
func NewFeistelCipher() *feistelCipher {
	f := &feistelCipher{}
	f.keySchedule(FeistelKey[:])
	return f
}

// roundFunction applies the Feistel round function
func (f *feistelCipher) roundFunction(state []byte, k []byte) {
	v5 := k[1] ^ state[1]
	v6 := k[2] ^ state[2]
	v7 := state[3] ^ k[3]
	v10 := k[0] ^ state[0]

	v8 := ((v5 >> 7) | (v5 << 1)) & 0xFF
	v9 := ((v6 >> 5) | (v6 << 3)) & 0xFF
	v11 := ((v7 >> 2) | (v7 << 6)) & 0xFF
	v12 := v10 ^ v9 ^ v8
	v13 := SBox[(v11^v12)&0xff]

	state[0] = ((((v10 << 7) & 0xff) | (v11 >> 1)) + ((v13 >> 5) | ((v13 << 3) & 0xff))) & 0xff
	state[1] = (((v10 >> 1) | ((v8 << 7) & 0xff)) + ((v13 >> 6) | ((v13 << 2) & 0xff))) & 0xff
	state[2] = (((v8 >> 1) | ((v9 << 7) & 0xff)) + ((v13 >> 7) | ((v13 << 1) & 0xff))) & 0xff
	state[3] = (((v9 >> 1) | ((v11 << 7) & 0xff)) + v13) & 0xff
}

// keySchedule generates subkeys from the master key
func (f *feistelCipher) keySchedule(key []byte) {
	v26 := make([]byte, 8)
	for i := 0; i < 7; i++ {
		v26[i] = key[i]
	}
	v26[1] ^= key[7]
	v26[4] ^= key[7]

	for i := range f.subkeys {
		f.subkeys[i] = 0
	}
	copy(f.subkeys[:7], v26[:7])

	v22 := make([]byte, 4)
	copy(v22, KeyScheduleSeed[:])

	for i := 0; i < 16; i++ {
		off := (i & 7) * 4
		f.roundFunction(f.subkeys[off:off+4], v22)
		copy(v22, f.subkeys[off:off+4])
	}
}

// DecryptBlock decrypts a single 8-byte block
func (f *feistelCipher) DecryptBlock(data []byte) {
	v6, v7, v5, v4 := data[0], data[1], data[2], data[3]
	v24, v25, v26, v27 := v6, v7, v5, v4
	v10, v11, v12, v13 := data[4], data[5], data[6], data[7]
	var v15, v16, v17, v18 byte

	for r := 15; r >= 0; r-- {
		st := []byte{v24, v25, v26, v27}
		f.roundFunction(st, f.subkeys[(r&7)*4:(r&7)*4+4])
		v24, v25, v26, v27 = st[0], st[1], st[2], st[3]

		v15 = v10 ^ v24
		v16 = v11 ^ v25
		v17 = v12 ^ v26
		v18 = v13 ^ v27

		v24 ^= v10
		v25 ^= v11
		v26 ^= v12
		v27 ^= v13

		v10, v11, v12, v13 = v6, v7, v5, v4

		if r == 0 {
			break
		}
		v6, v7, v5, v4 = v15, v16, v17, v18
	}

	data[0], data[1], data[2], data[3] = v6, v7, v5, v4
	data[4], data[5], data[6], data[7] = v15, v16, v17, v18
}

// EncryptBlock encrypts a single 8-byte block
func (f *feistelCipher) EncryptBlock(data []byte) {
	L := []byte{data[0], data[1], data[2], data[3]}
	R := []byte{data[4], data[5], data[6], data[7]}

	for r := 0; r < 16; r++ {
		st := make([]byte, 4)
		copy(st, L)
		f.roundFunction(st, f.subkeys[(r&7)*4:(r&7)*4+4])

		newR := make([]byte, 4)
		for i := 0; i < 4; i++ {
			newR[i] = R[i] ^ st[i]
		}

		copy(R, L)
		copy(L, newR)
	}

	// Swap L and R for final output
	data[0], data[1], data[2], data[3] = R[0], R[1], R[2], R[3]
	data[4], data[5], data[6], data[7] = L[0], L[1], L[2], L[3]
}

// Decrypt decrypts data in-place using Feistel cipher (8-byte blocks)
func (f *feistelCipher) Decrypt(data []byte) {
	for i := 0; i+8 <= len(data); i += 8 {
		f.DecryptBlock(data[i : i+8])
	}
}

// Encrypt encrypts data in-place using Feistel cipher (8-byte blocks)
func (f *feistelCipher) Encrypt(data []byte) {
	for i := 0; i+8 <= len(data); i += 8 {
		f.EncryptBlock(data[i : i+8])
	}
}

// AESDecryptCBC decrypts data using AES-128-CBC with the firmware keys
func AESDecryptCBC(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(AESKey[:])
	if err != nil {
		return nil, err
	}

	// Truncate to block boundary
	length := len(ciphertext) &^ 15
	if length == 0 {
		return []byte{}, nil
	}

	plaintext := make([]byte, length)
	iv := make([]byte, 16)
	copy(iv, AESIV[:])

	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(plaintext, ciphertext[:length])

	return plaintext, nil
}

// AESEncryptCBC encrypts data using AES-128-CBC with the firmware keys
func AESEncryptCBC(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(AESKey[:])
	if err != nil {
		return nil, err
	}

	// Truncate to block boundary
	length := len(plaintext) &^ 15
	if length == 0 {
		return []byte{}, nil
	}

	ciphertext := make([]byte, length)
	iv := make([]byte, 16)
	copy(iv, AESIV[:])

	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, plaintext[:length])

	return ciphertext, nil
}

// DecryptFirmware performs full AES decryption of firmware data
func DecryptFirmware(data []byte) ([]byte, error) {
	return AESDecryptCBC(data)
}

// EncryptFirmware performs full AES encryption of firmware data
func EncryptFirmware(data []byte) ([]byte, error) {
	return AESEncryptCBC(data)
}
