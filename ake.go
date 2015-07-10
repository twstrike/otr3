package otr3

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"hash"
	"io"
	"math/big"
)

// AKE is authenticated key exchange context
type AKE struct {
	ourKey              *PrivateKey
	Rand                io.Reader
	r                   [16]byte
	x, y                *big.Int
	gx, gy              *big.Int
	protocolVersion     uint16
	senderInstanceTag   uint32
	receiverInstanceTag uint32
	revealKey, sigKey   akeKeys
	ssid                [8]byte
	myKeyID             uint32
}

type akeKeys struct {
	c      [16]byte
	m1, m2 [32]byte
}

const (
	msgTypeDHCommit  = 2
	msgTypeDHKey     = 10
	msgTypeRevealSig = 17
	msgTypeSig       = 18
)

func (ake *AKE) rand() io.Reader {
	if ake.Rand != nil {
		return ake.Rand
	}
	return rand.Reader
}

func (ake *AKE) generateRand() (*big.Int, error) {
	var randx [40]byte
	_, err := io.ReadFull(ake.rand(), randx[:])
	if err != nil {
		return nil, err
	}
	return new(big.Int).SetBytes(randx[:]), nil
}

func (ake *AKE) encryptedGx() ([]byte, error) {
	_, err := io.ReadFull(ake.rand(), ake.r[:])

	aesCipher, err := aes.NewCipher(ake.r[:])
	if err != nil {
		return nil, err
	}

	var gxMPI = appendMPI([]byte{}, ake.gx)
	ciphertext := make([]byte, len(gxMPI))
	iv := ciphertext[:aes.BlockSize]
	stream := cipher.NewCTR(aesCipher, iv)
	stream.XORKeyStream(ciphertext, gxMPI)

	return ciphertext, nil
}

func (ake *AKE) hashedGx() []byte {
	out := sha256.Sum256(ake.gx.Bytes())
	return out[:]
}

func (ake *AKE) calcAKEKeys(s *big.Int) {
	secbytes := appendMPI(nil, s)
	h := sha256.New()
	copy(ake.ssid[:], h2(0x00, secbytes, h)[:8])
	copy(ake.revealKey.c[:], h2(0x01, secbytes, h)[:16])
	copy(ake.sigKey.c[:], h2(0x01, secbytes, h)[16:])
	copy(ake.revealKey.m1[:], h2(0x02, secbytes, h))
	copy(ake.revealKey.m2[:], h2(0x03, secbytes, h))
	copy(ake.sigKey.m1[:], h2(0x04, secbytes, h))
	copy(ake.sigKey.m2[:], h2(0x05, secbytes, h))
}

func h2(b byte, secbytes []byte, h hash.Hash) []byte {
	h.Reset()
	var p [1]byte
	p[0] = b
	h.Write(p[:])
	h.Write(secbytes[:])
	out := h.Sum(nil)
	return out[:]
}

func (ake *AKE) calcDHSharedSecret(xKnown bool) *big.Int {
	if xKnown {
		return new(big.Int).Exp(ake.gy, ake.x, p)
	}
	return new(big.Int).Exp(ake.gx, ake.y, p)
}

func (ake *AKE) generateEncryptedSignature(key *akeKeys, xFirst bool) []byte {
	verifyData := ake.generateVerifyData(xFirst)
	mb := sumHMAC(key.m1[:], verifyData)
	xb := ake.calcXb(key, mb, xFirst)
	return appendData(nil, xb)
}

func (ake *AKE) generateVerifyData(xFirst bool) []byte {
	var verifyData []byte

	if xFirst {
		verifyData = appendMPI(verifyData, ake.gx)
		verifyData = appendMPI(verifyData, ake.gy)
	} else {
		verifyData = appendMPI(verifyData, ake.gy)
		verifyData = appendMPI(verifyData, ake.gx)
	}

	publicKey := ake.ourKey.PublicKey.serialize()
	verifyData = append(verifyData, publicKey...)
	return appendWord(verifyData, ake.myKeyID)
}

func sumHMAC(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)

	return mac.Sum(nil)
}

func (ake *AKE) calcXb(key *akeKeys, mb []byte, xFirst bool) []byte {
	var sigb []byte
	xb := ake.ourKey.PublicKey.serialize()
	xb = appendWord(xb, ake.myKeyID)

	sigb, _ = ake.ourKey.sign(ake.rand(), mb)
	xb = append(xb, sigb...)

	aesCipher, err := aes.NewCipher(key.c[:])
	if err != nil {
		panic(err.Error())
	}

	var iv [aes.BlockSize]byte
	ctr := cipher.NewCTR(aesCipher, iv[:])
	ctr.XORKeyStream(xb, xb)

	return xb
}

func (ake *AKE) dhCommitMessage() ([]byte, error) {
	var out []byte
	ake.myKeyID = 0

	x, err := ake.generateRand()
	if err != nil {
		return nil, err
	}

	ake.x = x
	ake.gx = new(big.Int).Exp(g1, ake.x, p)
	encryptedGx, err := ake.encryptedGx()
	if err != nil {
		return nil, err
	}

	out = appendShort(out, ake.protocolVersion)
	out = append(out, msgTypeDHCommit)
	out = appendWord(out, ake.senderInstanceTag)
	out = appendWord(out, ake.receiverInstanceTag)
	out = appendData(out, encryptedGx)
	out = appendData(out, ake.hashedGx())

	return out, nil
}

func (ake *AKE) dhKeyMessage() ([]byte, error) {
	var out []byte
	y, err := ake.generateRand()

	if err != nil {
		return nil, err
	}
	ake.y = y
	ake.gy = new(big.Int).Exp(g1, ake.y, p)
	out = appendShort(out, ake.protocolVersion)
	out = append(out, msgTypeDHKey)
	out = appendWord(out, ake.senderInstanceTag)
	out = appendWord(out, ake.receiverInstanceTag)
	out = appendMPI(out, ake.gy)

	return out, nil
}

func (ake *AKE) revealSigMessage() []byte {
	s := ake.calcDHSharedSecret(true)
	ake.calcAKEKeys(s)
	var out []byte
	out = appendShort(out, ake.protocolVersion)
	out = append(out, msgTypeRevealSig)
	out = appendWord(out, ake.senderInstanceTag)
	out = appendWord(out, ake.receiverInstanceTag)
	out = appendData(out, ake.r[:])
	encryptedSig := ake.generateEncryptedSignature(&ake.revealKey, true)
	macSig := sumHMAC(ake.revealKey.m2[:], encryptedSig)
	out = append(out, encryptedSig...)
	out = append(out, macSig[:20]...)
	return out
}

func (ake *AKE) sigMessage() []byte {
	s := ake.calcDHSharedSecret(false)
	ake.calcAKEKeys(s)
	var out []byte
	out = appendShort(out, ake.protocolVersion)
	out = append(out, msgTypeSig)
	out = appendWord(out, ake.senderInstanceTag)
	out = appendWord(out, ake.receiverInstanceTag)

	encryptedSig := ake.generateEncryptedSignature(&ake.sigKey, false)
	macSig := sumHMAC(ake.sigKey.m2[:], encryptedSig)
	out = append(out, encryptedSig...)
	out = append(out, macSig[:20]...)
	return out
}
