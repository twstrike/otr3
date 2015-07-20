package otr3

import "errors"

type smpStateExpect1 struct{}
type smpStateExpect2 struct{}
type smpStateExpect3 struct{}
type smpStateExpect4 struct{}

var errUnexpectedMessage = errors.New("unexpected SMP message")

type smpMessage interface {
	receivedMessage(*smpContext) smpMessage
	tlv() []byte
}

type smpState interface {
	receiveMessage1(*smpContext, smpMessage1) (smpState, smpMessage)
	receiveMessage2(*smpContext, smpMessage2) (smpState, smpMessage)
	receiveMessage3(*smpContext, smpMessage3) (smpState, smpMessage)
	receiveMessage4(*smpContext, smpMessage4) (smpState, smpMessage)
	receiveAbortMessage(*smpContext, smpMessageAbort) (smpState, smpMessage)
}

func (c *smpContext) receive(message []byte) []byte {
	var toSend smpMessage

	//TODO if msgState != encrypted
	//must abandon SMP state and restart the state machine

	m, ok := parseTLV(message)
	if !ok {
		// TODO: errors
		return nil
	}

	toSend = m.receivedMessage(c)

	if toSend != nil {
		return toSend.tlv()
	}

	return nil
}

func (smpStateExpect1) receiveMessage1(c *smpContext, m smpMessage1) (smpState, smpMessage) {
	err := c.verifySMPStartParameters(m)
	if err != nil {
		//TODO errors
		return smpStateExpect1{}, smpMessageAbort{}
	}

	ret, ok := c.generateSMPSecondParameters(c.secret, m)
	if !ok {
		//TODO error
		return smpStateExpect1{}, smpMessageAbort{}
	}

	return smpStateExpect3{}, ret.msg
}

func (smpStateExpect2) receiveMessage1(c *smpContext, m smpMessage1) (smpState, smpMessage) {
	return smpStateExpect1{}, smpMessageAbort{}
}

func (smpStateExpect3) receiveMessage1(c *smpContext, m smpMessage1) (smpState, smpMessage) {
	return smpStateExpect1{}, smpMessageAbort{}
}

func (smpStateExpect4) receiveMessage1(c *smpContext, m smpMessage1) (smpState, smpMessage) {
	return smpStateExpect1{}, smpMessageAbort{}
}

func (smpStateExpect1) receiveMessage2(c *smpContext, m smpMessage2) (smpState, smpMessage) {
	return smpStateExpect1{}, smpMessageAbort{}
}

func (smpStateExpect2) receiveMessage2(c *smpContext, m smpMessage2) (smpState, smpMessage) {
	return smpStateExpect4{}, nil
}

func (smpStateExpect3) receiveMessage2(c *smpContext, m smpMessage2) (smpState, smpMessage) {
	return smpStateExpect1{}, smpMessageAbort{}
}

func (smpStateExpect4) receiveMessage2(c *smpContext, m smpMessage2) (smpState, smpMessage) {
	return smpStateExpect1{}, smpMessageAbort{}
}

func (smpStateExpect1) receiveMessage3(c *smpContext, m smpMessage3) (smpState, smpMessage) {
	return smpStateExpect1{}, smpMessageAbort{}
}

func (smpStateExpect2) receiveMessage3(c *smpContext, m smpMessage3) (smpState, smpMessage) {
	return smpStateExpect1{}, smpMessageAbort{}
}

func (smpStateExpect3) receiveMessage3(c *smpContext, m smpMessage3) (smpState, smpMessage) {
	return smpStateExpect1{}, nil
}

func (smpStateExpect4) receiveMessage3(c *smpContext, m smpMessage3) (smpState, smpMessage) {
	return smpStateExpect1{}, smpMessageAbort{}
}

func (smpStateExpect1) receiveMessage4(c *smpContext, m smpMessage4) (smpState, smpMessage) {
	return smpStateExpect1{}, smpMessageAbort{}
}

func (smpStateExpect2) receiveMessage4(c *smpContext, m smpMessage4) (smpState, smpMessage) {
	return smpStateExpect1{}, smpMessageAbort{}
}

func (smpStateExpect3) receiveMessage4(c *smpContext, m smpMessage4) (smpState, smpMessage) {
	return smpStateExpect1{}, smpMessageAbort{}
}

func (smpStateExpect4) receiveMessage4(c *smpContext, m smpMessage4) (smpState, smpMessage) {
	return smpStateExpect1{}, nil
}

func (smpStateExpect1) receiveAbortMessage(c *smpContext, m smpMessageAbort) (smpState, smpMessage) {
	return smpStateExpect1{}, nil
}

func (smpStateExpect2) receiveAbortMessage(c *smpContext, m smpMessageAbort) (smpState, smpMessage) {
	return smpStateExpect1{}, nil
}

func (smpStateExpect3) receiveAbortMessage(c *smpContext, m smpMessageAbort) (smpState, smpMessage) {
	return smpStateExpect1{}, nil
}

func (smpStateExpect4) receiveAbortMessage(c *smpContext, m smpMessageAbort) (smpState, smpMessage) {
	return smpStateExpect1{}, nil
}

func (m smpMessage1) receivedMessage(c *smpContext) smpMessage {
	var ret smpMessage
	c.smpState, ret = c.smpState.receiveMessage1(c, m)
	return ret
}

func (m smpMessage2) receivedMessage(c *smpContext) smpMessage {
	var ret smpMessage
	c.smpState, ret = c.smpState.receiveMessage2(c, m)
	return ret
}

func (m smpMessage3) receivedMessage(c *smpContext) smpMessage {
	var ret smpMessage
	c.smpState, ret = c.smpState.receiveMessage3(c, m)
	return ret
}

func (m smpMessage4) receivedMessage(c *smpContext) smpMessage {
	var ret smpMessage
	c.smpState, ret = c.smpState.receiveMessage4(c, m)
	return ret
}

func (m smpMessageAbort) receivedMessage(c *smpContext) smpMessage {
	var ret smpMessage
	c.smpState, ret = c.smpState.receiveAbortMessage(c, m)
	return ret
}

func (smpStateExpect1) String() string { return "SMPSTATE_EXPECT1" }
func (smpStateExpect2) String() string { return "SMPSTATE_EXPECT2" }
func (smpStateExpect3) String() string { return "SMPSTATE_EXPECT3" }
