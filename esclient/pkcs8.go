package esclient

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/des"
	"crypto/pbkdf2"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"fmt"
	"hash"
)

// Decryption of private keys in the encrypted PKCS#8 format ("BEGIN
// ENCRYPTED PRIVATE KEY", RFC 5958) — what OpenSSL writes by default, since
// 1.1.0, for a key protected by a passphrase. The standard library reads
// PKCS#8 keys but not encrypted ones, and has no plan to; what is needed of
// RFC 8018 is small enough to be written here rather than pulled in as a
// dependency.
//
// Supported: the PBES2 scheme with a key derived by PBKDF2 (HMAC-SHA1,
// -SHA224, -SHA256, -SHA384 or -SHA512) and encryption by AES-128/192/256
// or triple DES, in CBC mode — every combination "openssl pkcs8 -topk8 -v2"
// produces. Not supported, and reported as such: keys derived with scrypt
// (which the standard library doesn't provide), and the older PBES1 and
// PKCS#12 schemes.

var (
	oidPBES2  = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 13}
	oidPBKDF2 = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 12}
	oidScrypt = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 11591, 4, 11}

	oidHMACWithSHA1   = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 7}
	oidHMACWithSHA224 = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 8}
	oidHMACWithSHA256 = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 9}
	oidHMACWithSHA384 = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 10}
	oidHMACWithSHA512 = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 11}

	oidAES128CBC  = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 2}
	oidAES192CBC  = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 22}
	oidAES256CBC  = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 42}
	oidDESEDE3CBC = asn1.ObjectIdentifier{1, 2, 840, 113549, 3, 7}
)

// maxPBKDF2Iterations bounds the work a key file can ask for: OpenSSL writes
// a few thousand to a few hundred thousand iterations, and a count far
// beyond that is a corrupted (or hostile) file that would otherwise keep the
// connection attempt busy for as long as it says.
const maxPBKDF2Iterations = 50_000_000

// errWrongPassphrase is what decryption with the wrong passphrase looks
// like: no error from the cipher, only a result that isn't a key.
var errWrongPassphrase = errors.New("wrong passphrase (or corrupted key)")

// The ASN.1 structures of RFC 5958 (EncryptedPrivateKeyInfo) and RFC 8018
// (PBES2-params, PBKDF2-params).
type (
	encryptedPrivateKeyInfo struct {
		Algorithm pkix.AlgorithmIdentifier
		Data      []byte
	}
	pbes2Params struct {
		KeyDerivation pkix.AlgorithmIdentifier
		Encryption    pkix.AlgorithmIdentifier
	}
	pbkdf2Params struct {
		Salt       []byte
		Iterations int
		KeyLength  int                      `asn1:"optional"`
		PRF        pkix.AlgorithmIdentifier `asn1:"optional"`
	}
)

// decryptPKCS8 decrypts the content of an "ENCRYPTED PRIVATE KEY" PEM block
// and returns the PKCS#8 key it holds, still DER-encoded (what a "PRIVATE
// KEY" block carries).
func decryptPKCS8(der []byte, passphrase string) ([]byte, error) {
	malformed := errors.New("malformed encrypted PKCS#8 key")

	var info encryptedPrivateKeyInfo
	if rest, err := asn1.Unmarshal(der, &info); err != nil || len(rest) != 0 {
		return nil, malformed
	}
	if !info.Algorithm.Algorithm.Equal(oidPBES2) {
		return nil, fmt.Errorf("PKCS#8 key encrypted with an unsupported scheme (%s): only PBES2 is supported, as written by \"openssl pkcs8 -topk8 -v2 aes-256-cbc\"", info.Algorithm.Algorithm)
	}
	var scheme pbes2Params
	if rest, err := asn1.Unmarshal(info.Algorithm.Parameters.FullBytes, &scheme); err != nil || len(rest) != 0 {
		return nil, malformed
	}

	switch {
	case scheme.KeyDerivation.Algorithm.Equal(oidPBKDF2):
	case scheme.KeyDerivation.Algorithm.Equal(oidScrypt):
		return nil, errors.New("PKCS#8 key derived with scrypt, which is not supported: only PBKDF2 is, as written by \"openssl pkcs8 -topk8 -v2 aes-256-cbc\" without -scrypt")
	default:
		return nil, fmt.Errorf("PKCS#8 key derived with an unsupported function (%s): only PBKDF2 is supported", scheme.KeyDerivation.Algorithm)
	}
	var kdf pbkdf2Params
	if rest, err := asn1.Unmarshal(scheme.KeyDerivation.Parameters.FullBytes, &kdf); err != nil || len(rest) != 0 {
		return nil, malformed
	}
	if kdf.Iterations <= 0 || kdf.Iterations > maxPBKDF2Iterations {
		return nil, fmt.Errorf("PKCS#8 key with an unreasonable PBKDF2 iteration count (%d)", kdf.Iterations)
	}
	newHash, err := pbkdf2Hash(kdf.PRF.Algorithm)
	if err != nil {
		return nil, err
	}

	var keyLength int
	newCipher := aes.NewCipher
	switch cipherOID := scheme.Encryption.Algorithm; {
	case cipherOID.Equal(oidAES128CBC):
		keyLength = 16
	case cipherOID.Equal(oidAES192CBC):
		keyLength = 24
	case cipherOID.Equal(oidAES256CBC):
		keyLength = 32
	case cipherOID.Equal(oidDESEDE3CBC):
		keyLength, newCipher = 24, des.NewTripleDESCipher
	default:
		return nil, fmt.Errorf("PKCS#8 key encrypted with an unsupported cipher (%s): AES-128/192/256-CBC and DES-EDE3-CBC are supported", cipherOID)
	}
	if kdf.KeyLength != 0 && kdf.KeyLength != keyLength {
		return nil, malformed
	}
	var iv []byte
	if rest, err := asn1.Unmarshal(scheme.Encryption.Parameters.FullBytes, &iv); err != nil || len(rest) != 0 {
		return nil, malformed
	}

	key, err := pbkdf2.Key(newHash, passphrase, kdf.Salt, kdf.Iterations, keyLength)
	if err != nil {
		return nil, err
	}
	block, err := newCipher(key)
	if err != nil {
		return nil, err
	}
	if len(iv) != block.BlockSize() || len(info.Data) == 0 || len(info.Data)%block.BlockSize() != 0 {
		return nil, malformed
	}
	plain := make([]byte, len(info.Data))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, info.Data)

	// CBC has no integrity check of its own: the padding, then the key
	// structure itself, are what tell a right passphrase from a wrong one.
	plain, ok := stripPKCS7Padding(plain, block.BlockSize())
	if !ok {
		return nil, errWrongPassphrase
	}
	if _, err := x509.ParsePKCS8PrivateKey(plain); err != nil {
		return nil, errWrongPassphrase
	}
	return plain, nil
}

// pbkdf2Hash returns the hash PBKDF2's pseudo-random function is built on.
// No identifier at all means HMAC-SHA1, the default of RFC 8018.
func pbkdf2Hash(prf asn1.ObjectIdentifier) (func() hash.Hash, error) {
	switch {
	case len(prf) == 0, prf.Equal(oidHMACWithSHA1):
		return sha1.New, nil
	case prf.Equal(oidHMACWithSHA224):
		return sha256.New224, nil
	case prf.Equal(oidHMACWithSHA256):
		return sha256.New, nil
	case prf.Equal(oidHMACWithSHA384):
		return sha512.New384, nil
	case prf.Equal(oidHMACWithSHA512):
		return sha512.New, nil
	}
	return nil, fmt.Errorf("PKCS#8 key derived with an unsupported PBKDF2 function (%s): HMAC-SHA1, -SHA224, -SHA256, -SHA384 and -SHA512 are supported", prf)
}

// stripPKCS7Padding removes the padding of a decrypted CBC payload: n bytes
// of value n, 1 <= n <= blockSize. ok is false if it isn't there, which is
// what decrypting with the wrong key most often yields.
func stripPKCS7Padding(data []byte, blockSize int) (stripped []byte, ok bool) {
	if len(data) == 0 {
		return nil, false
	}
	n := int(data[len(data)-1])
	if n == 0 || n > blockSize || n > len(data) {
		return nil, false
	}
	for _, b := range data[len(data)-n:] {
		if int(b) != n {
			return nil, false
		}
	}
	return data[:len(data)-n], true
}
