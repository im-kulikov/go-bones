package network

import (
	"github.com/im-kulikov/go-bones/service"
)

type queueSettings struct{}

type QueueOption func(*queueSettings)

func NewQueue(queue, outbox interface{}, options ...QueueOption) service.Service {
	return nil
}
