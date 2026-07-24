package mq

import (
	"fmt"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	ExchangeBooking = "booking.ex"
	QueueBooking    = "booking.created.q"
	RoutingKey      = "booking.created"
)

type MQClient struct {
	conn    *amqp.Connection
	channel *amqp.Channel
}

func NewMQClient(url string) (*MQClient, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to rabbitmq: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}

	// Declare Topic Exchange
	err = ch.ExchangeDeclare(
		ExchangeBooking, // name
		"topic",         // type
		true,            // durable
		false,           // auto-deleted
		false,           // internal
		false,           // no-wait
		nil,             // arguments
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to declare exchange: %w", err)
	}

	// Declare Queue
	q, err := ch.QueueDeclare(
		QueueBooking, // name
		true,         // durable
		false,        // delete when unused
		false,        // exclusive
		false,        // no-wait
		nil,          // arguments
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to declare queue: %w", err)
	}

	// Bind Queue to Exchange
	err = ch.QueueBind(
		q.Name,          // queue name
		RoutingKey,      // routing key
		ExchangeBooking, // exchange
		false,
		nil,
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to bind queue: %w", err)
	}

	return &MQClient{conn: conn, channel: ch}, nil
}

func (mq *MQClient) Publish(routingKey string, body []byte) error {
	return mq.channel.Publish(
		ExchangeBooking, // exchange
		routingKey,      // routing key
		false,           // mandatory
		false,           // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         body,
		},
	)
}

func (mq *MQClient) Consume(handler func(body []byte)) error {
	msgs, err := mq.channel.Consume(
		QueueBooking, // queue
		"",           // consumer
		true,         // auto-ack
		false,        // exclusive
		false,        // no-local
		false,        // no-wait
		nil,          // args
	)
	if err != nil {
		return err
	}

	go func() {
		for d := range msgs {
			handler(d.Body)
		}
	}()

	log.Printf("📥 [RabbitMQ Consumer] Lắng nghe tin nhắn từ queue '%s'...", QueueBooking)
	return nil
}

func (mq *MQClient) Close() {
	if mq.channel != nil {
		mq.channel.Close()
	}
	if mq.conn != nil {
		mq.conn.Close()
	}
}
