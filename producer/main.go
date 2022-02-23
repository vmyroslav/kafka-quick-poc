package main

import (
	"encoding/json"
	"fmt"
	"github.com/bxcodec/faker/v3"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"log"
	"time"
)

var (
	server = "localhost:9092"
	topic  = "poc-test-part-1"

	seedSize = 1000
)

func main() {
	p, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": server,
		"security.protocol": "PLAINTEXT",
	})
	if err != nil {
		log.Fatalln(fmt.Errorf("can't initialize kafka producer %w", err))
	}
	defer p.Close()

	// Delivery report handler for produced messages
	waitChannel := make(chan struct{})
	go func() {
		var counter = 0
		for e := range p.Events() {
			switch e.(type) {
			case *kafka.Message:
				counter++
				if counter == seedSize {
					waitChannel <- struct{}{}
				}
			}
		}
	}()

	for i := 0; i < seedSize; i++ {
		m := NewMessage()
		m.Number = i
		m.Published = time.Now().UnixMicro()
		body, _ := json.Marshal(m)

		p.ProduceChannel() <- &kafka.Message{
			TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: int32(1)},
			Value:          body,
			Key:            []byte(m.ID),
		}
	}

	<-waitChannel
	log.Print("success")
}

type message struct {
	ID        string `faker:"uuid_hyphenated" json:"id"`
	Email     string `faker:"email" json:"email"`
	FirstName string `faker:"first_name" json:"first_name"`
	LastName  string `faker:"last_name" json:"last_name"`
	Number    int    `json:"number"`
	Published int64  `json:"published_at"`
}

func NewMessage() *message {
	m := message{}
	faker.FakeData(&m)

	return &m
}
