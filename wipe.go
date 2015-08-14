package otr3

import "math/big"

func (p *dhKeyPair) wipe() {
	if p == nil {
		return
	}

	wipeBigInt(p.pub)
	wipeBigInt(p.priv)
	p.pub = nil
	p.priv = nil
}

func (k *akeKeys) wipe() {
	if k == nil {
		return
	}

	wipeBytes(k.c[:])
	wipeBytes(k.m1[:])
	wipeBytes(k.m2[:])
}

func (a *ake) wipe() {
	if a == nil {
		return
	}

	wipeBigInt(a.secretExponent)
	a.secretExponent = nil

	wipeBigInt(a.ourPublicValue)
	a.ourPublicValue = nil

	wipeBigInt(a.theirPublicValue)
	a.theirPublicValue = nil

	wipeBytes(a.r[:])

	a.wipeGX()
	a.revealKey.wipe()
	a.sigKey.wipe()
}

func (a *ake) wipeGX() {
	if a == nil {
		return
	}

	wipeBytes(a.hashedGx[:])
	wipeBytes(a.encryptedGx[:])
	a.encryptedGx = nil
}

func (c *keyManagementContext) wipeKeys() {
	if c == nil {
		return
	}

	c.ourCurrentDHKeys.wipe()
	c.ourPreviousDHKeys.wipe()

	wipeBigInt(c.theirCurrentDHPubKey)
	c.theirCurrentDHPubKey = nil

	wipeBigInt(c.theirPreviousDHPubKey)
	c.theirPreviousDHPubKey = nil
}

func (c *keyManagementContext) wipe() {
	if c == nil {
		return
	}

	c.wipeKeys()
	c.ourCounter = 0
	c.ourKeyID = 0
	c.theirKeyID = 0
	keyPairCounters := make([]keyPairCounter, len(c.keyPairCounters))
	copy(c.keyPairCounters, keyPairCounters)
	c.keyPairCounters = nil

	for i := range c.oldMACKeys {
		c.oldMACKeys[i].wipe()
		c.oldMACKeys[i] = macKey{}
	}
	c.oldMACKeys = nil

	c.macKeyHistory.wipe()
}

func (c *keyManagementContext) wipeAndKeepRevealKeys() keyManagementContext {
	ret := keyManagementContext{}
	ret.oldMACKeys = make([]macKey, len(c.oldMACKeys))
	copy(ret.oldMACKeys, c.oldMACKeys)

	c.wipe()

	return ret
}

func (h *macKeyHistory) wipe() {
	if h == nil {
		return
	}

	for i := range h.items {
		h.items[i].wipe()
		h.items[i] = macKeyUsage{} //prevent memory leak
	}
	h.items = nil
}

func (u *macKeyUsage) wipe() {
	if u == nil {
		return
	}

	u.receivingKey.wipe()
	u.receivingKey = macKey{}
}

func (k *macKey) wipe() {
	if k == nil {
		return
	}

	wipeBytes(k[:])
}

func zeroes(n int) []byte {
	return make([]byte, n)
}

func wipeBytes(b []byte) {
	copy(b, zeroes(len(b)))
}

func wipeBigInt(k *big.Int) {
	if k == nil {
		return
	}

	k.SetBytes(zeroes(len(k.Bytes())))
}

func setBigInt(dst *big.Int, src *big.Int) *big.Int {
	wipeBigInt(dst)

	ret := big.NewInt(0)
	ret.Set(src)
	return ret
}
