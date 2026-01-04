package common

type MessageContext struct {
	Message   *MessagePayload
	ack       func() error
	ackCalled bool
}

func (c *MessageContext) Ack() error {
	if c.ack != nil {
		c.ackCalled = true
		return c.ack()
	}
	return nil
}

func (c *MessageContext) WasControlCalled() bool {
	return c.ackCalled
}
