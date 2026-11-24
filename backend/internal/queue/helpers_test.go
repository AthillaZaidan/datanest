package queue

import (
	amqp "github.com/rabbitmq/amqp091-go"
)

func deliveryWithBody(body string) amqp.Delivery {
	return amqp.Delivery{Body: []byte(body)}
}
