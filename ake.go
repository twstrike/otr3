package otr3

import (
	"crypto/hmac"
	"crypto/subtle"
	"io"
	"math/big"
	"time"

	"github.com/coyim/constbn"
)

var dontIgnoreFastRepeatQueryMessage = "false"

type ake struct {
	secretExponent   secretKeyValue
	ourPublicValue   *big.Int
	theirPublicValue *big.Int

	// TODO: why this number here?
	// A random value r of 128 bits (16 byte)
	r [16]byte

	encryptedGx []byte

	// SIZE: this should always be version.hash2Length
	xhashedGx []byte

	revealKey akeKeys
	sigKey    akeKeys

	state authState
	keys  keyManagementContext

	lastStateChange time.Time
}

func (c *Conversation) ensureAKE() {
	if c.ake != nil {
		return
	}

	c.initAKE()
}

func (c *Conversation) initAKE() {
	c.ake = &ake{
		state: authStateNone{},
	}
}

func (c *Conversation) calcAKEKeys(s *big.Int) {
	c.ssid, c.ake.revealKey, c.ake.sigKey = calculateAKEKeys(s, c.version)
}

func (c *Conversation) setSecretExponent(val secretKeyValue) {
	c.ake.secretExponent = createSecretKeyValue(val)
	c.ake.ourPublicValue = modExpPCT(g1ct, val).GetBigInt()
}

func (c *Conversation) calcDHSharedSecret() *big.Int {
	return modExpPCT(new(constbn.Int).SetBigInt(c.ake.theirPublicValue), c.ake.secretExponent).GetBigInt()
}

func (c *Conversation) generateEncryptedSignature(key *akeKeys) ([]byte, error) {
	verifyData := appendAll(c.ake.ourPublicValue, c.ake.theirPublicValue, c.ourCurrentKey.PublicKey(), c.ake.keys.ourKeyID)

	mb := sumHMAC(key.m1, verifyData, c.version)
	xb, err := c.calcXb(key, mb)

	if err != nil {
		return nil, err
	}

	return AppendData(nil, xb), nil
}
func appendAll(one, two *big.Int, publicKey PublicKey, keyID uint32) []byte {
	return AppendWord(append(AppendMPI(AppendMPI(nil, one), two), publicKey.serialize()...), keyID)
}

func fixedSizeCopy(s int, v []byte) []byte {
	vv := make([]byte, s)
	copy(vv, v)
	return vv
}

func (c *Conversation) calcXb(key *akeKeys, mb []byte) ([]byte, error) {
	xb := c.ourCurrentKey.PublicKey().serialize()
	xb = AppendWord(xb, c.ake.keys.ourKeyID)

	sigb, err := c.ourCurrentKey.Sign(c.rand(), mb)
	if err == io.ErrUnexpectedEOF || err == io.EOF {
		return nil, errShortRandomRead
	}

	if err != nil {
		return nil, err
	}

	// We have to copy the key here, and expand it to the requisite length -
	// not all c values will be long enough. We immediately clean the copy
	// afterwards
	ccopy := fixedSizeCopy(c.version.keyLength(), key.c)
	tryLock(ccopy)
	defer func() {
		tryUnlock(ccopy)
		wipeBytes(ccopy)
	}()

	// this error can't happen, since key.c is fixed to the correct size
	xb, _ = encrypt(ccopy, append(xb, sigb...))

	return xb, nil
}

// dhCommitMessage = bob = x
// Bob ---- DH Commit -----------> Alice
func (c *Conversation) dhCommitMessage() ([]byte, error) {
	c.initAKE()
	c.ake.keys.ourKeyID = 0

	// A random value x of at least 320 bits (40 byte)
	x, err := c.randSecret(make([]byte, 40))
	if err != nil {
		return nil, err
	}

	c.setSecretExponent(x)
	wipeSecretKeyValue(x)

	if err := c.randomInto(c.ake.r[:]); err != nil {
		return nil, err
	}

	// this can't return an error, since ake.r is of a fixed size that is always correct
	// we send in a slice here, since the original r is a fixed array. the slicing process
	// does NOT create a copy of the data
	c.ake.encryptedGx, _ = encrypt(c.ake.r[:], AppendMPI(nil, c.ake.ourPublicValue))

	return c.serializeDHCommit(c.ake.ourPublicValue), nil
}

func (c *Conversation) serializeDHCommit(public *big.Int) []byte {
	dhCommitMsg := dhCommit{
		encryptedGx: c.ake.encryptedGx,
		yhashedGx:   c.version.hash2(AppendMPI(nil, public)),
	}
	return dhCommitMsg.serialize()
}

// dhKeyMessage = alice = y
// Alice -- DH Key --------------> Bob
func (c *Conversation) dhKeyMessage() ([]byte, error) {
	c.initAKE()

	// A random value x of at least 320 bits (40 byte)
	y, err := c.randSecret(make([]byte, 40)[:])

	if err != nil {
		return nil, err
	}

	c.setSecretExponent(y)
	wipeSecretKeyValue(y)

	return c.serializeDHKey(), nil
}

func (c *Conversation) serializeDHKey() []byte {
	dhKeyMsg := dhKey{
		gy: c.ake.ourPublicValue,
	}

	return dhKeyMsg.serialize()
}

// revealSigMessage = bob = x
// Bob ---- Reveal Signature ----> Alice
func (c *Conversation) revealSigMessage() ([]byte, error) {
	c.calcAKEKeys(c.calcDHSharedSecret())
	c.ake.keys.ourKeyID++

	encryptedSig, err := c.generateEncryptedSignature(&c.ake.revealKey)
	if err != nil {
		return nil, err
	}

	macSig := sumHMAC(c.ake.revealKey.m2, encryptedSig, c.version)
	revealSigMsg := revealSig{
		r:            c.ake.r,
		encryptedSig: encryptedSig,
		macSig:       macSig,
	}

	return revealSigMsg.serialize(c.version), nil
}

// sigMessage = alice = y
// Alice -- Signature -----------> Bob
func (c *Conversation) sigMessage() ([]byte, error) {
	c.ake.keys.ourKeyID++

	encryptedSig, err := c.generateEncryptedSignature(&c.ake.sigKey)
	if err != nil {
		return nil, err
	}

	macSig := sumHMAC(c.ake.sigKey.m2, encryptedSig, c.version)
	sigMsg := sig{
		encryptedSig: encryptedSig,
		macSig:       macSig,
	}

	return sigMsg.serialize(c.version), nil
}

// processDHCommit = alice = y
// Bob ---- DH Commit -----------> Alice
func (c *Conversation) processDHCommit(msg []byte) error {
	dhCommitMsg := dhCommit{}
	err := dhCommitMsg.deserialize(msg)
	if err != nil {
		return err
	}

	c.ake.encryptedGx = dhCommitMsg.encryptedGx
	c.ake.xhashedGx = dhCommitMsg.yhashedGx

	return err
}

// processDHKey = bob = x
// Alice -- DH Key --------------> Bob
func (c *Conversation) processDHKey(msg []byte) (isSame bool, err error) {
	dhKeyMsg := dhKey{}
	err = dhKeyMsg.deserialize(msg)
	if err != nil {
		return false, err
	}

	if !isGroupElement(dhKeyMsg.gy) {
		return false, newOtrError("DH value out of range")
	}

	//If receive same public key twice, just retransmit the previous Reveal Signature
	if c.ake.theirPublicValue != nil {
		isSame = eq(c.ake.theirPublicValue, dhKeyMsg.gy)
		return
	}

	c.ake.theirPublicValue = dhKeyMsg.gy
	return
}

// processRevealSig = alice = y
// Bob ---- Reveal Signature ----> Alice
func (c *Conversation) processRevealSig(msg []byte) (err error) {
	revealSigMsg := revealSig{}
	err = revealSigMsg.deserialize(msg, c.version)
	if err != nil {
		return
	}

	r := revealSigMsg.r[:]
	theirMAC := revealSigMsg.macSig
	encryptedSig := revealSigMsg.encryptedSig

	decryptedGx := make([]byte, len(c.ake.encryptedGx))
	if err = decrypt(r, decryptedGx, c.ake.encryptedGx); err != nil {
		return
	}

	if err = checkDecryptedGx(decryptedGx, c.ake.xhashedGx, c.version); err != nil {
		return
	}

	if c.ake.theirPublicValue, err = extractGx(decryptedGx); err != nil {
		return
	}

	c.calcAKEKeys(c.calcDHSharedSecret())
	if err = c.processEncryptedSig(encryptedSig, theirMAC, &c.ake.revealKey); err != nil {
		return newOtrError("in reveal signature message: " + err.Error())
	}

	return nil
}

// processSig = bob = x
// Alice -- Signature -----------> Bob
func (c *Conversation) processSig(msg []byte) (err error) {
	sigMsg := sig{}
	err = sigMsg.deserialize(msg)
	if err != nil {
		return
	}

	theirMAC := sigMsg.macSig
	encryptedSig := sigMsg.encryptedSig

	if err := c.processEncryptedSig(encryptedSig, theirMAC, &c.ake.sigKey); err != nil {
		return newOtrError("in signature message: " + err.Error())
	}

	return nil
}

func (c *Conversation) checkedSignatureVerification(mb, sig []byte) error {
	rest, ok := c.theirKey.Verify(mb, sig)
	if !ok {
		return newOtrError("bad signature in encrypted signature")
	}

	if len(rest) > 0 {
		return errCorruptEncryptedSignature
	}

	return nil
}

func verifyEncryptedSignatureMAC(encryptedSig []byte, theirMAC []byte, keys *akeKeys, v otrVersion) error {
	tomac := AppendData(nil, encryptedSig)

	myMAC := sumHMAC(keys.m2, tomac, v)[:v.truncateLength()]

	if len(myMAC) != len(theirMAC) || subtle.ConstantTimeCompare(myMAC, theirMAC) == 0 {
		return newOtrError("bad signature MAC in encrypted signature")
	}

	return nil
}

func (c *Conversation) parseTheirKey(key []byte) (sig []byte, keyID uint32, err error) {
	var rest []byte
	var ok, ok2 bool
	rest, ok, c.theirKey = ParsePublicKey(key)
	sig, keyID, ok2 = ExtractWord(rest)
	if !(ok && ok2) {
		return nil, 0, errCorruptEncryptedSignature
	}

	return
}

func (c *Conversation) expectedMessageHMAC(keyID uint32, keys *akeKeys) []byte {
	verifyData := appendAll(c.ake.theirPublicValue, c.ake.ourPublicValue, c.theirKey, keyID)
	return sumHMAC(keys.m1, verifyData, c.version)
}

func (c *Conversation) processEncryptedSig(encryptedSig []byte, theirMAC []byte, keys *akeKeys) error {
	if err := verifyEncryptedSignatureMAC(encryptedSig, theirMAC, keys, c.version); err != nil {
		return err
	}

	decryptedSig := encryptedSig

	// We have to copy the key here, and expand it to the requisite length -
	// not all c values will be long enough. We immediately clean the copy
	// afterwards
	ccopy := fixedSizeCopy(c.version.keyLength(), keys.c)
	tryLock(ccopy)
	defer func() {
		tryUnlock(ccopy)
		wipeBytes(ccopy)
	}()

	if err := decrypt(ccopy, decryptedSig, encryptedSig); err != nil {
		return err
	}

	sig, keyID, err := c.parseTheirKey(decryptedSig)
	if err != nil {
		return err
	}

	mb := c.expectedMessageHMAC(keyID, keys)
	if err := c.checkedSignatureVerification(mb, sig); err != nil {
		return err
	}

	c.ake.keys.theirKeyID = keyID

	return nil
}

func extractGx(decryptedGx []byte) (*big.Int, error) {
	newData, gx, ok := ExtractMPI(decryptedGx)
	if !ok || len(newData) > 0 {
		return gx, newOtrError("gx corrupt after decryption")
	}

	if !isGroupElement(gx) {
		return gx, newOtrError("DH value out of range")
	}

	return gx, nil
}

func sumHMAC(key, data []byte, v otrVersion) []byte {
	mac := hmac.New(v.hash2Instance, key)
	_, _ = mac.Write(data)
	return mac.Sum(nil)
}

func checkDecryptedGx(decryptedGx, hashedGx []byte, v otrVersion) error {
	digest := v.hash2(decryptedGx)

	if subtle.ConstantTimeCompare(digest[:], hashedGx[:]) == 0 {
		return newOtrError("bad commit MAC in reveal signature message")
	}

	return nil
}
